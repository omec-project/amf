// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

// Package lawfulintercept is the AMF's Lawful Interception IRI-POI. It receives
// interception tasks over X1 (mutual TLS), matches AMF events against tasked
// targets, and delivers the resulting xIRI to an MDF2 over X2. It is opt-in:
// inactive — and silent — unless the AMF is started with LI credentials, so an
// AMF that is not intercepting behaves and looks exactly like one without it.
package lawfulintercept

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/nasconv"
	"github.com/omec-project/nas/v2"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/li/iri"
	"github.com/omec-project/util/li/mtls"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/omec-project/util/li/x2x3"
)

// Config configures the AMF LI IRI-POI. Init is only called when LI is enabled.
type Config struct {
	// BlockError, when non-nil, is why the element's `li` configuration block was refused
	// before it reached here — an unrecognised key (most likely a misspelling of one this
	// element does model) or a value of the wrong type.
	//
	// It is carried rather than acted on by the loader because refusing a configuration is not
	// the same as refusing to run: the network function has work of its own, and stopping it
	// over an optional subsystem is both an outage and the loudest way to disclose that this
	// element is LI-provisioned. Interception does not start on it; the AMF does.
	BlockError error

	X1Listen string // address for the X1 provisioning listener, e.g. ":8443"
	MDF2     string // default X2 destination ("host:port") for a task naming no destination
	NEID     string // this network element's identifier (echoed in X1 responses)
	Cert     string // X0-pre-provisioned LI PKI: this NE's certificate
	Key      string //                            its private key
	CACert   string //                            the LI CA trust anchor

	// Destinations are DID→endpoint mappings this element can resolve without their
	// having been provisioned over X1, for destinations agreed out of band.
	Destinations []Destination

	AdmfURL string // ADMF X1 endpoint for NE-initiated issue reports (empty = disabled)
	AdmfID  string // the responsible ADMF's identifier: authenticates inbound X1 peers and addresses outbound reports (empty accepts any certified ADMF)
	// KeepaliveTimeout, KeepaliveTimeP1 and KeepaliveTimeP3 are the timers of the X1
	// keepalive procedure (TS 103 221-1 clause 6.6.2) as the operator wrote them:
	// TIME_P2, TIME_P1 and TIME_P3. Empty takes the clause default (1h, 1m, 2h). The
	// procedure runs unless KeepaliveDisabled is set, because the clause enables it by
	// default; KeepaliveRetainTasking keeps it from ever deactivating tasking.
	//
	// Strings rather than durations, and parsed inside Init, because a value this
	// element cannot read has to be *reported* — and the only channel it may be
	// reported on is the one the reporter opens, which does not exist until Init runs.
	KeepaliveTimeout       string
	KeepaliveTimeP1        string
	KeepaliveTimeP3        string
	KeepaliveRetainTasking bool
	KeepaliveDisabled      bool

	// The three settings of the TS 103 221-2 clause 6.2.4 keepalive mechanism, as the
	// operator wrote them. Parsed here rather than by the caller because an unusable
	// value is reported to the ADMF over X1, and the reporter does not exist until
	// this subsystem starts.
	X2X3KeepaliveEnabled *bool
	X2X3KeepaliveTimeP1  string
	X2X3KeepaliveTimeP2  string

	// DeactivateAllTasks and RemoveAllDestinations are the two bulk operations
	// TS 103 221-1 leaves to advance agreement between the operator and the agency.
	// Nil is "no agreement in advance" and leaves the standard's defaults, which
	// li/x1 holds; see x1.BulkOptions.
	DeactivateAllTasks    *bool
	RemoveAllDestinations *bool
}

// Destination is one pre-shared delivery destination: a DID an ADMF's tasks reference,
// and where it points. It resolves exactly as a destination provisioned over X1 does; a
// provisioned entry for the same DID wins.
type Destination struct {
	DID          string
	DeliveryType string // X2Only | X3Only | X2andX3
	Address      string // host:port
}

// amfInterceptionPoint is the Interception Point ID every xIRI from this element
// carries (ETSI TS 103 221-2 clause 5.3.8): it names the POI within the network
// function, and this network function contains exactly one — the IRI-POI that reports
// mobility events.
const amfInterceptionPoint = "AMF-IRI-POI"

// errNoElementIdentifier means the deployment configured interception without the
// identifier this element asserts on X1, which is also the Network Function ID every
// record it delivers has to carry (TS 33.128 table 5.3.1-2).
var errNoElementIdentifier = errors.New("li: no network element identifier configured")

// errNoX1Listen means the deployment configured interception without an address for the
// X1 listener to bind.
//
// This is not caught by the bind: Listen(ctx, "tcp", "") *succeeds*, on every interface at
// an OS-selected ephemeral port. So interception starts, reports itself healthy, and waits
// for tasking on a port nobody — the ADMF least of all — can predict. The element is
// unprovisionable and indistinguishable from one that is merely un-tasked, which is the
// worst shape this failure could take: silent, and only visible once a warrant that was
// supposed to be running turns out never to have arrived.
var errNoX1Listen = errors.New("li: no X1 listen address configured")

// sender delivers an xIRI/xCC PDU to an MDF. *x2x3.Client satisfies it; tests
// inject a capturing implementation to assert per-warrant delivery isolation.
type sender interface {
	Send(*x2x3.PDU) error
}

type subsystem struct {
	// keepalive is the X1 keepalive procedure this element runs, or nil when the operator
	// disabled it.
	keepalive *x1.KeepaliveProcedure

	// scans counts the activation scans this subsystem has dispatched. Nothing in
	// production waits on it — the whole point of the scan is that a provisioning answer
	// does not — but a scan is this subsystem's work, and owning it is what lets a test wait
	// for the walk it started rather than leaving a goroutine to fail the next test.
	scans sync.WaitGroup

	store *store.Store
	// senderFor returns the delivery client for one X2 destination address. It is a
	// function rather than a single client because a task's destinations arrive over
	// X1: two warrants may name two agencies' MDF2s, and delivering both to one address
	// is cross-agency disclosure.
	senderFor func(addr string) sender
	// unreachable answers how many of the destinations this element's tasking currently
	// names cannot be reached, and how many of them it has attempted at all — the delivery
	// pool's accounting, scoped to what is in use (see destinationsInUse). A function
	// rather than the pool itself for the same reason senderFor is one: a test states a
	// delivery condition without an MDF to take away.
	unreachable func() (unreachable, inUse int)
	// unreachableAt answers the same question about one address, which is the shape a
	// destination-scoped answer needs: an element's own status says how much is wrong, a
	// destination's says whether that one is working. Same source, so the pushed report and
	// the interrogation cannot disagree.
	unreachableAt func(addr string) bool
	// mdf2 is the configured X2 endpoint. It serves a task that names no destination, and
	// nothing else: a task whose named destinations yield no X2 endpoint gets no xIRI rather
	// than this endpoint's (see x2Destinations).
	mdf2 string
	neID string
	// ids supplies the conditional attributes that belong to this element rather than
	// to the task — its two identities and the per-context sequence numbering — and is
	// shared with the SMF's IRI-POI and the UPF's CC-POI through li/x2x3.
	ids      *x2x3.Identity
	reporter *x1.Reporter // nil when NE-initiated reporting is not configured
	// policies holds the UE policy COMMANDs relayed to targets and not yet answered — see
	// NoteUEPolicyCommand.
	policies pendingPolicies
}

// deliveryFault is what this element can answer about itself when an ADMF asks for its
// status: whether the mediation functions it delivers to are reachable right now.
//
// Only the delivery clients know this — li/x1 holds the tasking, not the sockets — so
// without it the element would answer that no observable condition holds however long it had
// been failing to deliver. That answer is true and useless, and it is ignored exactly as
// fast as an element that always reports itself faulty.
//
// It answers from what the last delivery attempt established and dials nothing, because it
// runs on the X1 request goroutine: a probe that went looking would hold up a provisioning
// function's answer.
//
// It asks only about the destinations this element's *current* tasking names — see
// destinationsInUse — so a warrant's withdrawal takes its destination out of the answer with
// it.
//
// A subsystem with no delivery accounting reports nothing rather than panicking on the X1
// request path — an element that cannot say is not an element that is broken.
func (s *subsystem) deliveryFault() *x1.X1Error {
	if s.unreachable == nil {
		return nil
	}

	return x1.MDFUnreachableProbe(s.unreachable)()
}

// destinationUnreachable is whether delivery to one address is currently failing, for the
// destination status the provisioning function can ask for.
//
// A method rather than the field itself so that an element with no delivery pool — a test, or
// an element built before the pool exists — answers "reachable" rather than panicking. That
// answer is also the one the X1 layer defaults to, so the two agree.
func (s *subsystem) destinationUnreachable(addr string) bool {
	if s.unreachableAt == nil {
		return false
	}

	return s.unreachableAt(addr)
}

// destinationsInUse is where this element's xIRI currently goes: the X2 endpoints the tasking
// it holds names, and the configured MDF2 for a task that names no destination.
//
// It exists because a delivery client outlives the warrant that created it. A destination
// whose last delivery failed and whose warrant was then deactivated can never be delivered to
// again, so nothing would ever clear it — the element would report itself faulty for the life
// of the process, including while holding no tasking at all. Scoping the question to what is
// in use is what keeps that probe from sticking on.
func (s *subsystem) destinationsInUse() []string {
	var addrs []string
	for _, t := range s.store.Snapshot() {
		addrs = append(addrs, s.iriDestinations(t)...)
	}

	return addrs
}

// iriDestinations is where t's xIRI goes from this element: nowhere for a task that wants no
// xIRI, otherwise what x2Destinations resolves — the question the delivery path asks, so the
// status answer and the destination watcher describe the destinations product goes to.
func (s *subsystem) iriDestinations(t types.InterceptTask) []string {
	if !t.WantsProduct(types.ProductIRI) {
		return nil
	}
	return s.x2Destinations(t)
}

// x2Health is what the destination watcher samples: the X2 destinations the tasking delivers
// xIRI to, each with its reachability. It resolves through iriDestinations, so the configured
// MDF2 serving a task that named no DID is watched and an MDF2 no task delivers xIRI to is
// not. Latent here rather than live: canApply refuses a task wanting no xIRI, so none reaches
// this element's store through X1.
func (s *subsystem) x2Health(unreachableAt func(addr string) bool) []x1.DestinationHealth {
	return x1.DestinationHealthOf(s.store.Snapshot(), types.DeliveryX2, s.iriDestinations, unreachableAt)
}

// active holds the running subsystem, or nil when LI is not configured.
var active atomic.Pointer[subsystem]

// newX1Server builds this element's X1 provisioning endpoint from its configuration.
//
// Separate from Init so that what an operator's configuration does to the X1 server can be
// asserted against the server this element actually runs, rather than against a second
// copy of the same wiring written in a test — which is where a configured policy quietly
// stops being applied.
func newX1Server(st *store.Store, cfg Config, sub *subsystem) *x1.Server {
	// WithADMF holds X1 peers to the responsible ADMF's identity: a certificate
	// from the LI CA authenticates a peer, but only this identifier may task us
	// (TS 103 221-1 clause 8.2.4 + error 1040).
	// A peer that fails that check is refused, and — since this plane deliberately
	// logs nothing — would otherwise be refused in complete silence. The ADMF is the
	// only party entitled to hear that someone is trying to task its network
	// elements under an identity that is not theirs.
	opts := []x1.Option{
		x1.WithADMF(cfg.AdmfID),
		// The keepalive procedure's reports travel on the element's fault channel: the 9050
		// fault when the ADMF goes quiet, its clearing, and the 10000 alert after a purge.
		x1.KeepaliveReporter(sub.reporter),
		x1.WithConfiguredDestinations(configuredDestinations(cfg.Destinations, sub.reporter)...),
		// The conditions this POI can observe about itself, which li/x1 cannot: see
		// subsystem.deliveryFault.
		x1.WithFaultProbes(sub.deliveryFault),
		// And the same fact at destination scope, for GetDestinationDetails. Without it the
		// element answers activeAndWorking for an endpoint it is simultaneously reporting
		// as unreachable over ReportDestinationIssue — two statements about one fact, and
		// the ADMF trusts the one it asked for.
		x1.WithDestinationReachability(sub.destinationUnreachable),
		// **No x1.WithTaskFaults here, and that is a decision rather than an omission.**
		//
		// A task-scoped fault has to be a condition that stops *this* warrant's interception
		// and that this element can re-observe when asked. This POI produces records from
		// signalling events for a subject it resolves by subscriber identity, and the states it
		// can see are the wrong shape for that:
		//
		//   - A subject who is not attached, or is attached and doing nothing, produces no
		//     records — and is indistinguishable from one this element has stopped watching. A
		//     fault reported for that would fire for every warrant on a quiet subscriber, and a
		//     fault channel that fires when nothing is wrong is one an ADMF learns to ignore.
		//   - Delivery failing is real and is already reported, at the two scopes that can say
		//     something true about it: the element's own status, and the destination's. A task
		//     names destinations rather than owning them, so attributing an endpoint's failure
		//     to one of the warrants pointing at it would report the same fault once per
		//     warrant and imply each was separately broken.
		//
		// The triggered CC-POI is the element where this is different, and it is where the
		// supplier is registered: it holds per-task datapath state — whether a duplication rule
		// this task requires is programmed — and its triggering function has no other account of
		// that interception at all.
		//
		// So this element answers `provisioningStatus: complete` with an empty fault list for
		// every task it holds, which is a true statement about a POI whose observable faults are
		// not task-scoped. Recorded here so the next reader finds the reasoning rather than the
		// absence.
		x1.OnTaskChange(sub.applyTaskChange),
		// Refuse a warrant this element could never act on. It resolves subjects by
		// subscriber identity alone (see targetsOf), so a warrant naming only a UE
		// address, a tunnel endpoint or a port matches nothing here at every moment —
		// and acknowledging it tells the ADMF an interception is running that cannot
		// be. Producing nothing is also what a tasked subject who does nothing
		// produces, so the agency has no way to tell the two apart and waits.
		//
		// Refused only when *none* of the named identifiers is resolvable here: a
		// warrant naming a SUPI and a UE address is one this element can partly serve,
		// and declining it would refuse interception it is capable of performing.
		x1.CanApply(sub.canApplyHere),
		x1.OnAuthFailure(func(code int) {
			if sub.reporter == nil {
				return
			}
			// Off this goroutine: OnAuthFailure documents that it runs synchronously on
			// the X1 request goroutine and must not block, and Notify is a synchronous
			// HTTPS round trip to the ADMF. Reporting an authentication failure by
			// holding the provisioning interface open for the duration of a POST to a
			// peer that may itself be unreachable turns a refused request into a stalled
			// X1 channel — and makes the element's response time depend on whether the
			// ADMF is up, which is observable to whoever is probing it.
			//
			// The dispatch is bounded in effect rather than in count, by NotifyAsync:
			// the throttle is consulted on this goroutine before anything is spawned,
			// so under a flood of refusals each of these costs a mutex. Spawning first
			// would cost a goroutine per refusal, which at the UPF, where the
			// equivalent sites are driven by packet rate, is unbounded. One form, so
			// the three elements cannot reach three answers to this hazard.
			sub.reporter.NotifyAsync(x1.NEIssueX1AuthFailed,
				fmt.Sprintf("X1 provisioning refused: peer failed authentication (error %d)", code))
		}),
	}
	// The two bulk operations the standard settles by advance agreement rather than by
	// what the element is. Unset leaves its defaults; li/x1 owns what unset means.
	opts = append(opts, x1.BulkOptions(cfg.DeactivateAllTasks, cfg.RemoveAllDestinations)...)

	return x1.NewServer(st, cfg.NEID, opts...)
}

// Init starts the AMF LI IRI-POI: it loads the LI credentials, opens the X1
// listener (mutual TLS), and prepares X2 delivery to the MDF2. Call it once at
// AMF startup, only when LI is configured.
func Init(cfg Config) error {
	// Without an identifier for this network element, product would reach a mediation
	// function that cannot attribute it to the element that produced it — and an MDF
	// accepts such a delivery rather than refusing it. Interception does not start; the
	// AMF itself carries on serving traffic, because a network function that crash-loops
	// over its LI configuration tells every operator it is LI-provisioned.
	if cfg.NEID == "" {
		return errNoElementIdentifier
	}

	mat, err := mtls.Load(cfg.Cert, cfg.Key, cfg.CACert)
	if err != nil {
		return err
	}
	st := store.New()
	var reporter *x1.Reporter
	if cfg.AdmfURL != "" {
		reporter = x1.NewReporter(cfg.AdmfURL, cfg.AdmfID, cfg.NEID, mat.ClientTLS())
	}
	// The refused `li` block, now that there is somewhere to report it. Checked before any
	// value out of that block is used, because the block is exactly what could not be read:
	// the keys that did decode are not trustworthy as a set, and one of the ones that did not
	// may be a keepalive timer or the fault endpoint.
	if cfg.BlockError != nil {
		reporter.NotifyAsync(x1.NEIssueInvalidConfig,
			"this element's interception configuration carries a setting it does not "+
				"recognise or cannot read, so the values it would fall back on cannot be trusted; "+
				"interception has not been started")

		return cfg.BlockError
	}
	// The listener's address, before anything binds it. An empty address is a bind that
	// succeeds on an unpredictable port rather than one that fails, so nothing downstream
	// will ever report it — see errNoX1Listen. Reported to the ADMF, because an element that
	// cannot be provisioned is precisely what the ADMF needs to hear about, and refused,
	// because starting is what makes it look healthy.
	if cfg.X1Listen == "" {
		reporter.NotifyAsync(x1.NEIssueInvalidConfig,
			"no X1 listen address is configured, so this element would accept tasking on an "+
				"unpredictable port; interception has not been started")

		return errNoX1Listen
	}
	// The keepalive timers, read once the block itself is trusted. Interception does not
	// start on one this element cannot read — the element would otherwise run a procedure
	// other than the one agreed with the agency, and look healthy while it did — but the
	// network function does, because an element that refuses to run over its LI
	// configuration is distinguishable from one that has none by anybody who can see
	// whether it is running.
	keepalive, err := keepaliveProcedure(cfg)
	if err != nil {
		reporter.NotifyAsync(x1.NEIssueInvalidConfig,
			"the configured X1 keepalive timers are not durations this element can use, so "+
				"interception has not been started")

		return err
	}
	// Deliver X2 asynchronously: the Report* hooks run on the per-UE GMM/NAS
	// goroutine (some before the downlink NAS is even built), so a slow or
	// unreachable MDF2 must never block them — that would delay a targeted UE's
	// signalling, a target-observable timing side channel and an availability risk
	// — so delivery is asynchronous by design. Worker delivery failures
	// surface to the ADMF over X1 (throttled, NE-level, no target id), never a log.
	//
	// One client per destination address, created on first use: a task carries the
	// endpoints its product goes to, and TS 33.128 marks them mandatory, so this
	// element cannot know them at startup.
	// The delivery-failure hook does not report reachability itself. An unreachable
	// mediation function is a condition this element can re-observe — the pool's own
	// accounting answers it — so it has an ending, and TS 103 221-1 clause 5.3 requires
	// an ending to be reported too. A site that announces with nothing that retracts
	// eventually announces something nobody retracts, so both edges belong to whoever
	// can see the transition. The hook nudges that watcher, which keeps the report
	// prompt and the decision in one place.
	//
	// Both declared before the pool because its callbacks close over them, and both are
	// only *called* once delivery is under way — by which time each is assigned.
	var (
		watcher *x1.DestinationWatcher
		sub     *subsystem
	)
	pool, err := x2x3.NewPool(mat.ClientTLS(),
		keepaliveConfig(cfg, reporter),
		// The error is inspected, not discarded: see reportDeliveryError.
		func(err error) { sub.reportDeliveryError(err, watcher) },
		// Product dropped because the queue was full is reported as it happens, and
		// this hook is the only place that can report it. Queue saturation is
		// deliberately excluded from reachability (see AsyncSender.Unreachable): a full
		// queue at one instant is a burst the buffer exists to absorb rather than a fault
		// an ADMF can act on. Without this report a reachable but slow MDF2 loses xIRI
		// while the destination watcher goes on reporting it healthy — product missing
		// from an agency's record with every channel that could have said so reporting
		// normality.
		//
		// Off the offering path, which is a signalling goroutine: this fires exactly
		// when delivery is already behind, so blocking here would add the reporting
		// stall to the condition being reported.
		func() {
			reporter.NotifyAsync(x1.NEIssueX2DeliveryLost,
				"xIRI dropped from the X2 delivery queue")
		},
	)
	if err != nil {
		// NewPool refuses a nil TLS configuration and keepalive timers Validate refuses.
		// Neither reaches it today: the configuration comes from the loaded material, and
		// keepaliveConfig falls back to the defaults on timers it cannot use. Checked so
		// that a change to either refuses the start instead of failing every delivery.
		return fmt.Errorf("lawful interception: X2 delivery: %w", err)
	}
	sub = &subsystem{
		store:     st,
		senderFor: func(addr string) sender { return pool.For(addr) },
		mdf2:      cfg.MDF2,
		neID:      cfg.NEID,
		ids:       x2x3.NewIdentity(cfg.NEID, amfInterceptionPoint),
		reporter:  reporter,
	}
	// Assigned after construction because it reads the subsystem it belongs to: the pool
	// knows what each destination's last delivery established, and only the subsystem knows
	// which destinations the tasking still names.
	sub.unreachable = func() (int, int) { return pool.UnreachableAmong(sub.destinationsInUse()) }
	sub.unreachableAt = pool.UnreachableAt
	// The watcher's view of the same destinations, with the identifiers the ADMF
	// provisioned them under. A different shape from the probe's on purpose: the
	// probe answers a status request and takes counts so it *cannot* name a
	// destination, and a destination-scoped report says which. Same fact, two
	// questions.
	if reporter != nil {
		watcher = x1.NewDestinationWatcher(func() []x1.DestinationHealth {
			// What delivery resolves, not DeliveryAddresses: the configured MDF2
			// serving a task that named no DID is watched too.
			return sub.x2Health(func(addr string) bool { return pool.UnreachableAt(addr) })
		}, reporter, 0)
	}
	x1srv := newX1Server(st, cfg, sub)
	// Bind the X1 listener synchronously so a bind/permission failure is reported
	// to the caller — otherwise LI would look enabled (active.Store below) while
	// no X1 tasking can ever be received.
	// ListenConfig.Listen rather than net.Listen so the listen carries a context
	// (the linter's noctx rule); the bind is otherwise unchanged.
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", cfg.X1Listen)
	if err != nil {
		// Surface the failure to the ADMF over X1 too (an operational fault, not a
		// per-target signal), best-effort.
		if sub.reporter != nil {
			sub.reporter.NotifyAsync(x1.NEIssueX1ListenFailed, "X1 listener bind failed")
		}
		return fmt.Errorf("lawful interception: X1 listen on %s: %w", cfg.X1Listen, err)
	}
	// Started once the bind has succeeded, so a failed Init leaves no goroutine behind.
	if watcher != nil {
		go watcher.Watch(nil)
	}
	// NewListener supplies the properties every X1 endpoint needs and none of the
	// three network functions should be trusted to remember separately: a discarded
	// error log and per-phase timeouts, without which an unauthenticated peer can
	// hold connections open until this element can no longer be untasked.
	srv := x1.NewListener(x1srv, mat.ServerTLS())
	// Certificates come from TLSConfig, so the file arguments are empty. ServeTLS
	// blocks until the listener closes; the bind already succeeded above.
	//nolint:errcheck // serve-until-close; a bind failure already surfaced above
	go func() { _ = srv.ServeTLS(ln, "", "") }()
	// The X1 keepalive procedure (TS 103 221-1 clause 6.6.2): report the ADMF's silence,
	// and deactivate tasking it can no longer withdraw. A nil stop channel: it runs for as
	// long as this element can hold tasking.
	sub.keepalive = keepalive
	if keepalive != nil {
		go x1srv.WatchKeepalive(*keepalive, nil)
	}
	active.Store(sub)
	// Tasking lives in memory, so this element has just discarded every warrant it
	// held. Nothing else tells the ADMF that — it goes on believing the
	// interceptions it provisioned are running — and the standard's audit path is a
	// query it has to think to make. Saying so on the way up is the one push signal
	// available.
	if reporter != nil && st.Len() == 0 {
		// Asynchronously, like every other start-up report here. Notify is a synchronous mTLS
		// round trip bounded only by its own 10s client timeout, and Init runs inline in the
		// network function's Start — before NGAP, before the service-based interface, before
		// registration with the network. `st.Len() == 0` is true on every start, so a
		// synchronous report would delay *every* start-up of an LI-provisioned AMF by up to
		// ten seconds while the ADMF is unreachable.
		//
		// That is an undetectability failure and not merely a slow start: a network function
		// that comes up later when a third party is down is distinguishable from one that is
		// not LI-provisioned, by anyone who can see when it starts serving — and the signal is
		// produced by the outage rather than by anything this element logs, so no amount of care
		// about log content removes it.
		reporter.NotifyAsync(x1.NEIssueTaskingAbsent,
			"network function started with interception enabled and no tasking present")
	}
	return nil
}

// reportDeliveryError is the delivery pool's onError hook: what this element does with a
// failure its delivery worker reports.
//
// **The error is inspected, not discarded.** ErrUnitDropped says delivery to this
// destination is working and one product unit of it was lost — a unit the write stopped
// inside, which the library resends whole on a fresh connection and reports as dropped only
// where that resend did not land either. The library deliberately refuses to call that
// unreachability, because a healthy mediation function must not be reported unreachable over
// one truncated write. Discarded here, the loss would be reported by nothing at all while the
// watcher samples a destination it correctly considers reachable: product missing from an
// agency's record with every channel that could have said so reporting normality, which is
// the failure direction this whole plane exists to prevent.
//
// Reported as the same delivery loss a full queue is: from the agency's side the two are one
// fact — an xIRI this element produced and did not deliver.
//
// The nudge is for every error, this one included: what the sender concluded about
// reachability is its own business, and the watcher's job is to re-observe it promptly rather
// than one sampling interval later.
//
// A method rather than a closure inside Init so the mapping can be asserted directly. What
// produces ErrUnitDropped is the transport, and that is tested where it lives, in li/x2x3;
// what this element does with it is this function.
//
// The report says how many xIRIs the drop cost. A write that stops inside one unit is resent
// from that unit to the end of its batch, so a resend that does not land loses every unit in
// that tail, and the library counts them (x2x3.LostUnits). Saying "an xIRI" for what may be a
// run of them would leave the agency's gap larger than this element's own account of it.
func (s *subsystem) reportDeliveryError(err error, watcher *x1.DestinationWatcher) {
	if errors.Is(err, x2x3.ErrUnitDropped) {
		s.reporter.NotifyAsync(x1.NEIssueX2DeliveryLost, droppedXIRIs(x2x3.LostUnits(err)))
	}
	watcher.Nudge()
}

// droppedXIRIs describes a drop of n xIRIs on the way to a reachable mediation function. An
// error that carries no count (n is 0) still lost at least the unit it names, so it is not
// described as a number this element cannot back.
func droppedXIRIs(n int) string {
	switch {
	case n > 1:
		return fmt.Sprintf("%d xIRIs were partially written to a reachable mediation function and dropped", n)
	case n == 1:
		return "an xIRI was partially written to a reachable mediation function and dropped"
	default:
		return "at least one xIRI was partially written to a reachable mediation function and dropped"
	}
}

// ReportRegistration emits an AMFRegistration xIRI for ue if it matches an active
// interception task. Every registration type is an AMFRegistration, mobility registration
// updating included: TS 33.128 clause 6.2.2.2.4 reports the location a mobility
// registration yields in that registration's xIRI rather than in a separate
// AMFLocationUpdate. A task scoped to IdentifierAssociation receives no AMFRegistration
// (clause 6.2.2.2.1) but does receive AMFLocationUpdate, so for a mobility registration
// such a task gets an AMFLocationUpdate instead. It is a no-op when LI is inactive or ue
// is not a target, and never logs anything that would reveal interception.
//
// access is the access the registration completes on. It is part of the record's
// registrationResult, alongside any other access the UE is already registered over.
func ReportRegistration(ue *amfctx.AmfUe, access models.AccessType) {
	sub := active.Load()
	if sub == nil || ue == nil {
		return
	}
	id := ue.IdentitySnapshot()
	tasks, ids, at := sub.matchingTasks(id), targetsOf(id), time.Now()
	sub.deliverIRI(tasks, ids, at, x2x3.DirectionFromTarget, amfRegistration(id, registrationResultOf(ue, access)))
	if id.RegistrationType5GS != nasMessage.RegistrationType5GSMobilityRegistrationUpdating {
		return
	}
	var locationOnly []types.InterceptTask
	for _, t := range tasks {
		if !t.WantsGeneralRecords() {
			locationOnly = append(locationOnly, t)
		}
	}
	sub.deliverIRI(locationOnly, ids, at, x2x3.DirectionFromTarget, amfLocationUpdate(id))
}

// registrationResultOf is the 5GS registration result of TS 24.501 clause 9.11.3.6 for a
// registration completing on completedOn: that access, which is not yet in the Registered
// state when the record is made, and any other access ue is registered over. It is built as
// BuildRegistrationAccept builds the IE, and the NAS access bits are the record's
// AMFRegistrationResult values (1, 2, and 3 for both).
func registrationResultOf(ue *amfctx.AmfUe, completedOn models.AccessType) iri.AMFRegistrationResult {
	result := iri.RegResult3GPPAccess
	if completedOn == models.ACCESSTYPE_NON_3_GPP_ACCESS {
		result = iri.RegResultNon3GPPAccess
	}
	for _, other := range registeredAccesses(ue) {
		result |= other
	}

	return result
}

// ReportLocationUpdate emits an AMFLocationUpdate xIRI when NG-RAN tells the AMF the target has
// moved: the PATH SWITCH REQUEST of an Xn handover, the HANDOVER NOTIFY of an N2 handover, and
// the PDU SESSION RESOURCE MODIFY INDICATION NG-RAN sends when it activates or releases dual
// connectivity (TS 33.128 clause 6.2.2.2.4). Every task scope receives it (clause 6.2.2.2.1).
// NG-RAN initiates these procedures, so the record is "sent to the target". No-op and silent
// when LI is inactive or ue is not a target.
func ReportLocationUpdate(ue *amfctx.AmfUe) {
	sub := active.Load()
	if sub == nil || ue == nil {
		return
	}
	id := ue.IdentitySnapshot()
	sub.reportEvent(id, x2x3.DirectionToTarget, amfLocationUpdate(id))
}

// ReportServiceAccept emits an AMFUEServiceAccept xIRI when the AMF has sent a
// SERVICE ACCEPT to the target. Call it only where an accept was actually sent:
// a record for an accept the AMF failed to build is a false report.
//
// TS 33.128 also names a SERVICE ACCEPT answering a CONTROL PLANE SERVICE
// REQUEST, which this AMF does not implement, so only the plain event occurs.
// serviceType is the service type value the SERVICE REQUEST carried (TS 24.501
// clause 9.11.3.50). No-op and silent when LI is inactive or ue is not a target.
func ReportServiceAccept(ue *amfctx.AmfUe, serviceType uint8) {
	sub := active.Load()
	if sub == nil || ue == nil {
		return
	}
	id := ue.IdentitySnapshot()
	// The service request procedure is UE-initiated (TS 24.501 clause 5.6.1), paging being
	// only what may prompt it.
	sub.reportEvent(id, x2x3.DirectionFromTarget, amfServiceAccept(id, serviceType))
}

// Handover describes one handover as the AMF sees it when the HANDOVER REQUEST
// ACKNOWLEDGE arrives — the point where TS 33.128 places both handover records.
// The caller assembles it so this package never touches ngapType.
type Handover struct {
	UE *amfctx.AmfUe
	// AMFUENGAPID and RANUENGAPID are the source NG-RAN node's association, which the
	// HANDOVER COMMAND travels over; TargetAMFUENGAPID and TargetRANUENGAPID are the target
	// node's, which the HANDOVER REQUEST ACKNOWLEDGE carries.
	AMFUENGAPID       int64
	RANUENGAPID       int64
	TargetAMFUENGAPID int64
	TargetRANUENGAPID int64
	HandoverType      int64
	TargetToSource    []byte // from the acknowledge
	SourceToTarget    []byte // carried from HANDOVER REQUIRED
	PDUSessionID      int32
	CauseGroup        HandoverCauseGroup
	// CauseValue is the cause in the vocabulary of the *record*, not of NGAP. The two number
	// the same causes differently, so the caller maps it before setting this field.
	CauseValue int64
	// CauseSubstituted says the network gave a cause TS 33.128 has no value for, and CauseValue
	// is that group's `unspecified` standing in for it.
	//
	// It travels because the substituted value is itself a legitimate cause: without knowing
	// which of the two happened, this element cannot tell an agency the difference between a
	// network that gave no reason and one that gave a reason the record could not carry.
	CauseSubstituted bool
	// TypeSubstituted says the network gave a handover type TS 33.128 has no value for, and
	// HandoverType carries `intra5GS` instead.
	//
	// Reported for the same reason CauseSubstituted is, and with more urgency: the enumeration
	// has no "unspecified", so the substituted value is a positive claim about what kind of
	// handover this was rather than an admission that the reason is unknown.
	TypeSubstituted bool
	HasCause        bool
	HasPDUSessionID bool
	// HasSourceToTarget says HANDOVER REQUIRED's container was carried forward. A flag, like
	// HasCause, so that what was carried is stated rather than inferred from a length.
	HasSourceToTarget bool
}

// HandoverCauseGroup names which arm of the TS 33.128 HandoverCause CHOICE a
// cause belongs to. The AMF's NGAP cause is a CHOICE over the same five groups —
// but not over the same values, so the caller maps the value as well as the arm
// (see ngap.liCauseValue).
type HandoverCauseGroup int

const (
	CauseGroupRadioNetwork HandoverCauseGroup = iota
	CauseGroupTransport
	CauseGroupNAS
	CauseGroupProtocol
	CauseGroupMisc
)

// ReportHandoverCommand emits an AMFRANHandoverCommand xIRI when the AMF has sent
// a HANDOVER COMMAND to the source RAN node. All five of its members are known at
// that point.
func ReportHandoverCommand(h Handover) {
	sub := active.Load()
	if sub == nil || h.UE == nil {
		return
	}
	id := h.UE.IdentitySnapshot()
	// A handover is initiated by the network: the source NG-RAN node decides it.
	sub.reportEvent(id, x2x3.DirectionToTarget, iri.AMFRANHandoverCommand{
		UserIdentifiers:         userIdentifiers(id),
		AMFUENGAPID:             iri.AMFUENGAPID(h.AMFUENGAPID),
		RANUENGAPID:             iri.RANUENGAPID(h.RANUENGAPID),
		HandoverType:            iri.HandoverType(h.HandoverType),
		TargetToSourceContainer: iri.RANTargetToSourceContainer(append([]byte(nil), h.TargetToSource...)),
	})
}

// ReportHandoverRequest emits an AMFRANHandoverRequest xIRI. Despite the name,
// TS 33.128 clause 6.2.2.2.9.3 triggers it on the AMF *receiving* the HANDOVER
// REQUEST ACKNOWLEDGE, not on sending the request — which is why the cause and
// the source-to-target container have to be carried forward from HANDOVER
// REQUIRED.
//
// Eight members are mandatory. If any of the carried ones is missing the record
// cannot be completed: it is not emitted with a fabricated cause or container, and the
// warrant is told as recordNotProduced rather than left to find a handover missing. The
// acknowledge's target-to-source container may be empty — the AMF copies what the RAN sent,
// and TS 38.413 sets it no minimum length (gnbsim rel-2.3.0 sends it so) — and is reported
// as the RAN sent it.
func ReportHandoverRequest(h Handover) {
	sub := active.Load()
	if sub == nil || h.UE == nil {
		return
	}
	if !h.HasCause || !h.HasPDUSessionID || !h.HasSourceToTarget {
		sub.reportNotProduced(h.UE.IdentitySnapshot(), iri.AMFRANHandoverRequest{},
			"a member HANDOVER REQUIRED carries forward was not held when the acknowledge arrived")

		return
	}
	// A cause the record's definition cannot express is carried as that group's `unspecified`
	// and said out loud. The substituted value is a legitimate cause in its own right, so a
	// silent substitution would leave an agency unable to tell a network that gave no reason
	// from one that gave a reason this element could not carry.
	//
	// Element-scoped and naming nothing: what it says is which values this element can express,
	// which is a property of the element and of the release its record definitions come from.
	// Throttled by the reporter like every other condition, so a run of handovers carrying the
	// same unrepresentable cause produces one report per window rather than one per handover.
	if h.CauseSubstituted && sub.reporter != nil {
		sub.reporter.NotifyAsync(x1.NEIssueRecordValueSubstituted,
			"a handover cause this element's record definitions cannot express was carried as "+
				"the group's unspecified value")
	}
	// The handover type has no "unspecified" to fall back on, so the substitution asserts a
	// kind of handover rather than declining to. Said out loud for that reason.
	if h.TypeSubstituted && sub.reporter != nil {
		sub.reporter.NotifyAsync(x1.NEIssueRecordValueSubstituted,
			"a handover type this element's record definitions cannot express was carried as "+
				"intra5GS")
	}

	id := h.UE.IdentitySnapshot()
	// The identifiers of the association the acknowledge arrived over: table 6.2.2.2.9.3-1's
	// rANUENGAPID is the one "the AMF receives from the NG-RAN node", the target node here.
	sub.reportEvent(id, x2x3.DirectionToTarget, iri.AMFRANHandoverRequest{
		UserIdentifiers:               userIdentifiers(id),
		AMFUENGAPID:                   iri.AMFUENGAPID(h.TargetAMFUENGAPID),
		RANUENGAPID:                   iri.RANUENGAPID(h.TargetRANUENGAPID),
		HandoverType:                  iri.HandoverType(h.HandoverType),
		HandoverCause:                 handoverCause(h.CauseGroup, h.CauseValue),
		PDUSessionResourceInformation: iri.PDUSessionResourceInformation{PDUSessionID: iri.PDUSessionID(h.PDUSessionID)},
		TargetToSourceContainer:       iri.RANTargetToSourceContainer(append([]byte(nil), h.TargetToSource...)),
		SourceToTargetContainer:       iri.RANSourceToTargetContainer(append([]byte(nil), h.SourceToTarget...)),
	})
}

// handoverCause picks the CHOICE arm. The group is half the meaning: "radio
// network: handover desirable for radio reason" and "misc: hardware failure"
// describe very different events, and a value alone cannot tell them apart.
func handoverCause(group HandoverCauseGroup, value int64) any {
	switch group {
	case CauseGroupTransport:
		return iri.CauseTransport(value)
	case CauseGroupNAS:
		return iri.CauseNas(value)
	case CauseGroupProtocol:
		return iri.CauseProtocol(value)
	case CauseGroupMisc:
		return iri.CauseMisc(value)
	case CauseGroupRadioNetwork:
		return iri.CauseRadioNetwork(value)
	default:
		return iri.CauseRadioNetwork(value)
	}
}

// ReportDeregistration emits an AMFDeregistration xIRI for ue if it matches an
// active task. networkInitiated distinguishes a network-ordered deregistration
// from a UE-originating one; access is the access type being deregistered.
// identity is the 5GS mobile identity a UE-originating DEREGISTRATION REQUEST carried, and nil
// for a network-initiated deregistration: the record's sUCI is the "SUCI used in the
// deregistration" (TS 33.128 table 6.2.2.2.3-1), so it is present only when that identity is a
// SUCI, not whenever the UE registered with one.
// No-op and silent when LI is inactive or ue is not a target.
func ReportDeregistration(ue *amfctx.AmfUe, networkInitiated bool, access iri.AccessType, identity []byte) {
	sub := active.Load()
	if sub == nil || ue == nil {
		return
	}
	dir, payloadDir := iri.DirUEInitiated, x2x3.DirectionFromTarget
	if networkInitiated {
		dir, payloadDir = iri.DirNetworkInitiated, x2x3.DirectionToTarget
	}
	id := ue.IdentitySnapshot()
	sub.reportEvent(id, payloadDir, amfDeregistration(id, dir, access, identity))
}

// ReportAbortedRegistration reports a registration procedure that ends with access
// deregistered, which matters when the procedure began from the Registered state: the UE was
// registered on access and no longer is, so TS 33.128 clause 6.2.2.2.3 reports an
// AMFDeregistration, network-initiated since the network ended the registration. When the UE
// is left registered on no access its SUPI to 5G-GUTI binding is released, and clause
// 6.2.2.2.7 reports an AMFIdentifierDeassociation.
//
// removed says the UE context is being removed, which ends every access it holds: a
// registration on the other access ends with it whatever this procedure began from, and is
// reported with it.
//
// Call it where the procedure fails, before the state is written. A procedure that began
// from Deregistered reports nothing for its own access, and each procedure reports once: the
// record of where it began is consumed here, whether or not LI is active. Silent, and reports
// nothing when LI is inactive or ue is not a target.
func ReportAbortedRegistration(ue *amfctx.AmfUe, access models.AccessType, removed bool) {
	if ue == nil {
		return
	}
	// The record outlives a procedure that succeeded, so it counts only while one is under way.
	wasRegistered := ue.TakeRegisteredAtProcedureStart(access) && inRegistrationProcedure(ue, access)
	if active.Load() == nil {
		return
	}
	other := models.ACCESSTYPE_NON_3_GPP_ACCESS
	if access == models.ACCESSTYPE_NON_3_GPP_ACCESS {
		other = models.ACCESSTYPE__3_GPP_ACCESS
	}
	otherEnds := removed && registeredOn(ue, other)
	var scope iri.AccessType
	switch {
	case wasRegistered && otherEnds:
		scope = iri.AccessBoth
	case wasRegistered:
		scope = accessType(access)
	case otherEnds:
		scope = accessType(other)
	default:
		return
	}
	ReportDeregistration(ue, true, scope, nil)
	if removed || !registeredOn(ue, other) {
		ReportIdentifierDeassociation(ue)
	}
}

// DeregistrationScope maps the access a UE-originating deregistration asked for onto the
// record's own vocabulary.
//
// **The record can name both accesses, and must when both were deregistered.** TS 33.128
// table 6.2.2.2.3-1 makes accessType mandatory with cardinality 1, and its type admits
// three values — AccessType ::= ENUMERATED { threeGPPAccess(1), nonThreeGPPAccess(2),
// threeGPPandNonThreeGPPAccess(3) } — so "both" is a value of the single field rather
// than a reason to emit two records. The clause's own trigger text says the xIRI is
// generated when a UE "has deregistered from the 5GS over at least one access type",
// which is one record about a deregistration however many accesses it covered.
//
// It names the access the message *asked for*, not the one it *arrived on*:
// HandleDeregistrationRequest releases the SM contexts of both accesses when the requested
// type is AccessTypeBoth, and a record naming one would contradict what the same function
// did. That is not the declarable case of a mandatory field the element cannot populate —
// it is a populated field asserting something false, and nothing downstream distinguishes
// the two.
func DeregistrationScope(nasAccessType uint8) iri.AccessType {
	switch nasAccessType {
	case nasMessage.AccessTypeBoth:
		return iri.AccessBoth
	case nasMessage.AccessType3GPP:
		return iri.AccessThreeGPP
	case nasMessage.AccessTypeNon3GPP:
		return iri.AccessNonThreeGPP
	}

	// A value this element does not recognise: report the access it is handling rather
	// than inventing a scope. The caller passes the arrival access for this case.
	return 0
}

// AccessScope maps a serving-access value onto the record's vocabulary, for the paths
// where the element acts on exactly one access and there is nothing to ask.
func AccessScope(a models.AccessType) iri.AccessType { return accessType(a) }

// ReportRegistrationReject emits an AMFUnsuccessfulProcedure xIRI (failed
// procedure = registration) for ue if it matches an active task; cause is the
// 5GMM reject cause. At reject time ue may be only partially identified — if no
// target identifier is yet known it matches no task and nothing is emitted.
// No-op and silent when LI is inactive.
func ReportRegistrationReject(ue *amfctx.AmfUe, cause uint8) {
	sub := active.Load()
	if sub == nil || ue == nil {
		return
	}
	// TS 33.128 clause 6.2.2.2.6: an unsuccessful registration is reported only if the UE
	// has been successfully authenticated. A UE holding a valid NAS security context has
	// been: by this procedure's authentication and security mode, or by an earlier one
	// whose context protected this request. The context is marked available when the
	// SECURITY MODE COMMAND is built, so a UE for which none is built (NEA0 and NIA0
	// selected) holds none, and its rejects are not reported.
	if !ue.SecurityContextIsValid() {
		return
	}
	id := ue.IdentitySnapshot()
	sub.reportEvent(id, x2x3.DirectionFromTarget,
		amfUnsuccessfulRegistration(id, cause, gutiPresented(ue.RegistrationRequest)))
}

// gutiPresented is the 5G-GUTI the UE named as its 5GS mobile identity in req, composed as
// AllocateGutiToUe composes one, or "" when it named another identity. It is the 5G-GUTI a
// rejected registration used: a GUTI the AMF allocated in the procedure never reached the UE.
func gutiPresented(req *nasMessage.RegistrationRequest) string {
	if req == nil {
		return ""
	}
	buf := req.GetMobileIdentity5GSContents()
	// TS 24.501 figure 9.11.3.4.1: type of identity in bits 1 to 3 of the first octet, and
	// eleven octets in all.
	if len(buf) != 11 || buf[0]&0x07 != nasMessage.MobileIdentity5GSType5gGuti {
		return ""
	}
	_, guti := nasconv.GutiToString(buf)
	return guti
}

// ReportIdentifierAssociation emits an AMFIdentifierAssociation xIRI for ue if
// it matches an active task — the AMF has bound the target's SUPI to a 5G-GUTI
// (a GUTI carried to the UE in a Registration Accept). No-op and silent when LI
// is inactive or ue is not a target.
func ReportIdentifierAssociation(ue *amfctx.AmfUe) {
	sub := active.Load()
	if sub == nil || ue == nil {
		return
	}
	id := ue.IdentitySnapshot()
	sub.reportEvent(id, x2x3.DirectionNotApplicable, amfIdentifierAssociation(id))
}

// ReportIdentifierDeassociation emits an AMFIdentifierDeassociation xIRI for ue
// if it matches an active task — the AMF has released the target's SUPI↔5G-GUTI
// binding (on deregistration). No-op and silent when LI is inactive or ue is
// not a target.
func ReportIdentifierDeassociation(ue *amfctx.AmfUe) {
	sub := active.Load()
	if sub == nil || ue == nil {
		return
	}
	// Registered on no access, so no UE policy procedure it had open will complete.
	sub.policies.forget(ue)
	id := ue.IdentitySnapshot()
	sub.reportEvent(id, x2x3.DirectionNotApplicable, amfIdentifierDeassociation(id))
}

// applyTaskChange is the X1 lifecycle hook (x1.OnTaskChange): one event per
// transition of this element's tasking, carrying the task as it was and as it
// becomes. prev nil is an activation, next nil a removal, both a modification.
//
// The numbering state is released on removal, and on a modification that moves the
// delivery label — and on nothing else. It is keyed by the *delivery* XID, which is the
// provisioned ProductID where a task carries one, so a relabel does change it: the
// contexts under the superseded label are then stranded, because every record from that
// point carries the new one and nothing will number under the old contexts again. A
// modification that leaves the labelling alone must release nothing, since those
// contexts are the ones the modified task's own records are still using — the next
// record in one of them would repeat a sequence number the mediation function has
// already seen for that (XID, correlation) pair.
//
// The cost is that a retarget leaves the old target's contexts numbered until the
// task is removed, since the sequencer is keyed by (XID, correlation) and cannot
// say which of those a target no longer covered. That is the right way round: an
// unused counter costs a map entry until the warrant ends, while a repeated
// number is how a mediation function detects lost product, so it must mean loss
// and nothing else.
func (s *subsystem) applyTaskChange(prev, next *types.InterceptTask) {
	switch {
	case next == nil:
		// Numbering state belongs to the tasking that created it. Without this the
		// element keeps one sequence context per warrant for the life of the process.
		s.ids.Forget(parseXID(prev.DeliveryXID()))
	case prev == nil:
		s.reportStartOfInterception(*next, nil)
	default:
		// The superseded label's contexts, before the record below can number under the
		// new one. Forget rather than ForgetContext because at an IRI-POI one task is
		// one warrant, so every context under that label belonged to this task.
		if prev.DeliveryXID() != next.DeliveryXID() {
			s.ids.Forget(parseXID(prev.DeliveryXID()))
		}
		if !modificationCanProduceARecord(*prev, *next) {
			return
		}
		s.reportStartOfInterception(*next, prev)
	}
}

// modificationCanProduceARecord reports whether a modification could yield a
// start-of-interception record at all.
//
// **The scan is not free.** Each one launches a goroutine that walks the whole registered-UE
// pool, and for a modification that changed only the destinations, the record scope or the
// delivery labelling, `covered()` suppresses every record the walk produces — because a UE the
// previous task already covered is not one whose interception begins here, and for those
// modifications that is every UE. Bulk provisioning runs one such walk per warrant, concurrently.
//
// So the predicate is "this modification cannot yield a record", not "no newly covered
// targets". Three things decide it:
//
//   - the targets, because a UE the task did not cover before is one whose interception
//     begins now;
//   - the products, because adding IRI to a content-only task begins interception of a
//     product for subjects the task already covered;
//   - the record scope, because widening it makes records due that were previously excluded.
//
// Getting the predicate too narrow loses a start-of-interception record that is due; getting
// it too wide only costs a walk. That asymmetry is why the test is written as three
// equalities rather than as an attempt to be clever about what changed.
func modificationCanProduceARecord(prev, next types.InterceptTask) bool {
	if !slices.Equal(prev.Targets, next.Targets) {
		return true
	}
	if prev.WantsProduct(types.ProductIRI) != next.WantsProduct(types.ProductIRI) {
		return true
	}
	if prev.WantsProduct(types.ProductCC) != next.WantsProduct(types.ProductCC) {
		return true
	}

	return prev.RecordScope != next.RecordScope
}

// reportStartOfInterception runs when a task is newly activated or modified over
// X1. It scans the AMF UE pool and, for every already-registered UE the task
// targets, emits an AMFStartOfInterceptionWithRegisteredUE — so a warrant that
// arrives after the UE is already on the network still produces an initial record.
//
// already, when set, is the task this one replaces. A UE it already covered is not
// one whose interception begins here, so no record says it does.
func (s *subsystem) reportStartOfInterception(task types.InterceptTask, already *types.InterceptTask) {
	if !task.WantsProduct(types.ProductIRI) {
		return
	}
	// The event these records report is the *activation*, not the registration each UE
	// completed earlier, so one instant taken here is the right timestamp for all of
	// them. Sampling per record would time the scan.
	//
	// Taken before the scan is handed off, so running the scan on another goroutine
	// does not move the instant the records report.
	activated := time.Now()

	// Off the X1 request goroutine. The scan walks every UE this AMF holds, so on that
	// goroutine answering a provisioning function would take time proportional to the
	// registered-UE population — an element that answers a tasking request more slowly the
	// busier it is, which is observable to whoever is asking and is the provisioning-latency
	// form of the rule that keeps delivery off the signalling path. The SMF's equivalent,
	// scanSessions, does the same.
	//
	// **Bounded, because bulk provisioning launches one of these per warrant at once.**
	// TS 103 221-1's bulk operations and an ADMF restoring tasking after an element restart
	// both provision many warrants in quick succession, and each walk reads the whole UE
	// pool: unbounded, N warrants cost N concurrent full walks, on an element whose ordinary
	// job is answering NAS procedures for those same UEs. The bound turns that into a queue.
	//
	// The provisioning answer still does not wait on any of it, which is why the scan is off
	// the request goroutine at all — the slot is taken *inside* the goroutine, not before it.
	s.scans.Add(1)
	go func() {
		defer s.scans.Done()

		scanSlots <- struct{}{}
		defer func() { <-scanSlots }()

		s.scanRegisteredUEs(task, already, activated)
	}()
}

// scanSlots bounds how many activation scans walk the UE pool at once.
//
// Four rather than one: a single slot would serialise unrelated warrants behind one walk, and
// what matters is that the cost is bounded rather than that it is one. Package-level because
// the bound is a property of this element's UE pool, of which there is one.
var scanSlots = make(chan struct{}, 4)

// scanWalked is called when a scan begins walking the UE pool. Set only by tests; nil
// otherwise.
var scanWalked func()

// scanRegisteredUEs emits the start-of-interception records, one per registered access, for
// every already-registered UE a newly activated task targets. It runs off the X1 request
// goroutine; see the caller.
func (s *subsystem) scanRegisteredUEs(task types.InterceptTask, already *types.InterceptTask, activated time.Time) {
	// nil in production. It exists because the *walk* is what the predicate and the bound are
	// about, and the walk is not observable from its records: a modification that cannot
	// produce one could still walk the whole pool and have covered() throw everything away. See
	// TestAModificationThatCannotProduceARecordStartsNoScan.
	if scanWalked != nil {
		scanWalked()
	}

	// Each record's subject is a different UE, so the identities the header reports are
	// per record rather than per task — hence the identity list travels beside the event.
	type startRecord struct {
		event any
		ids   []types.TargetIdentifier
		// id is the subject this record is about, kept so the per-record re-read can
		// establish that the task still names it. Without it the revalidation could
		// answer only whether the task still exists.
		id amfctx.UeIdentity
	}
	var events []startRecord
	amfctx.AMF_Self().UePool.Range(func(_, value any) bool {
		ue, ok := value.(*amfctx.AmfUe)
		if !ok {
			return true
		}
		// This scan runs concurrently with the UEs' own NAS procedures — on its own
		// goroutine, which if anything widens the window rather than closing it —
		// so the identity fields must be read through IdentitySnapshot:
		// it takes them together under the identity lock the NAS Set* accessors
		// write under, which is what makes the read race-free. Reading
		// the fields directly here would race every registration in flight.
		// ue.State needs no such care: its keys are fixed at UE creation and each
		// *fsm.State guards its own transitions.
		id := ue.IdentitySnapshot()
		if !taskTargets(task, id) || covered(already, id) {
			return true
		}
		// TS 33.128 clause 6.2.2.2.5: a UE registered over both accesses draws one record for
		// each access type, each naming that access as its registration result. A UE
		// registered over neither draws none.
		for _, access := range registeredAccesses(ue) {
			events = append(events, startRecord{
				event: amfStartOfInterception(id, access), ids: targetsOf(id), id: id,
			})
		}
		return true
	})
	if len(events) == 0 {
		return
	}
	// Delivery is asynchronous (enqueue-and-return; see Init), so nothing here blocks
	// on the MDF; hand the built records to the delivery client.
	//
	// **The task is re-read before each record, and the record goes out under what the
	// store holds now.** The scan's duration is unbounded by design — it is off the X1
	// goroutine precisely so a provisioning answer does not scale with the registered-UE
	// population — so "the warrant was valid when this scan started" and "the warrant is
	// valid now" are two different statements, and only the second one authorises a
	// record. A DeactivateTask acknowledged mid-scan otherwise leaves records for a
	// withdrawn warrant still arriving, at destinations a ModifyTask may already have
	// replaced; and it is exactly the failure an agency cannot audit, because from
	// outside the element the withdrawal looks complete.
	//
	// Per record rather than per scan: a withdrawal that lands mid-scan has to stop the
	// remainder, not merely the next one.
	//
	// **What the re-read establishes is a list, and the subject is on it.** It answers that
	// the task still exists, and — because the record is built from `current` — which
	// products it wants and where its product goes. It must also answer whether the task
	// still names *this subject*: otherwise a ModifyTask that retargets a warrant mid-scan
	// leaves the remaining records describing the previous subject and delivers them under
	// the warrant's own identifier, to the new subject's agency. Well-formed, correctly
	// attributed, and about somebody the warrant no longer covers.
	//
	// So the check is here, beside the re-read, rather than in the record's own closure:
	// what revalidation establishes is existence, products, destinations *and subject*, and
	// a later omission from that list should read as a gap in a list.
	for _, rec := range events {
		current, held := s.store.Get(task.XID)
		if !held {
			return
		}
		if !taskTargets(current, rec.id) {
			continue
		}
		s.deliverIRI([]types.InterceptTask{current}, rec.ids, activated, x2x3.DirectionNotApplicable, rec.event)
	}
}

// covered reports whether task was already intercepting this UE's IRI.
func covered(task *types.InterceptTask, id amfctx.UeIdentity) bool {
	return task != nil && task.WantsProduct(types.ProductIRI) && taskTargets(*task, id)
}

// reportEvent delivers event to every task the subject's identity matches, as the
// identity and the clock stood when the event was observed. dir is the PDU's Payload
// Direction (see deliverIRI).
//
// The instant is taken here, at the hook, because the X2 Timestamp attribute is the
// time the *event* occurred (TS 33.128 table 5.3.2-2) rather than the time a PDU was
// built. These hooks run synchronously with the procedure they observe, so this is as
// close to the event as this element can get; sampling the clock further down would
// time the record instead, and a mediation function cannot tell the two apart.
func (s *subsystem) reportEvent(id amfctx.UeIdentity, dir x2x3.PayloadDirection, event any) {
	s.deliverIRI(s.matchingTasks(id), targetsOf(id), time.Now(), dir, event)
}

// anyTaskReceives reports whether some task matching id is to receive a record of event's
// type: one that takes IRI and whose scope includes that record's class. event serves only
// to name the record.
func (s *subsystem) anyTaskReceives(id amfctx.UeIdentity, event any) bool {
	class := recordClassOf(event)
	for _, t := range s.matchingTasks(id) {
		if t.WantsProduct(types.ProductIRI) && produces(t, class) {
			return true
		}
	}
	return false
}

// reportNotProduced tells each warrant that wants event's record class that a record the
// clause mandates for it was not produced, because the content it carries was not held when its
// trigger occurred. event is the zero value of the record that was not produced and serves
// only to name it. Like recordNotEncoded it is a task-scoped event, reported once per
// occurrence (throttled per warrant by the reporter) and never retracted, and it runs off the
// UE's NAS goroutine.
func (s *subsystem) reportNotProduced(id amfctx.UeIdentity, event any, why string) {
	class := recordClassOf(event)
	for _, t := range s.matchingTasks(id) {
		if !t.WantsProduct(types.ProductIRI) || !produces(t, class) {
			continue
		}
		s.reporter.NotifyTaskAsync(string(t.DeliveryXID()),
			x1.TaskReportNonTerminatingFault, x1.TaskIssueRecordNotProduced,
			fmt.Sprintf("%s: a %T record for this warrant was not produced: %s",
				x1.TaskIssueRecordNotProduced, event, why))
	}
}

// deliverIRI encodes event once and delivers it as an X2 xIRI to every task in
// tasks that wants IRI product, at the destinations that task named. It is silent
// on any error (encoding or delivery) so that interception can never be inferred
// from AMF behaviour.
//
// subjectIDs are the identities this AMF holds for the record's subject, which the
// header reports split into the ones each task matched and the rest; at is when the
// event happened.
//
// dir is the Payload Direction. TS 33.128 table 5.3.2-1 sets it, for an xIRI reporting a
// network procedure, from the procedure's initiator; in TS 103 221-2 table 5.2.6-1 terms a
// procedure the target UE initiated is 3 (sent from the target) and one the network
// initiated is 2 (sent to the target). The start-of-interception and identifier
// association and deassociation records are 5 (not applicable), as their clauses say.
func (s *subsystem) deliverIRI(tasks []types.InterceptTask, subjectIDs []types.TargetIdentifier, at time.Time, dir x2x3.PayloadDirection, event any) {
	if len(tasks) == 0 {
		return
	}
	if s.ids == nil {
		// An element that cannot say which network function produced a record does not
		// deliver one: the mediation function would accept product it cannot attribute.
		// Init always supplies this and refuses to start without the identifier behind
		// it, so reaching here means a subsystem was assembled by hand — fail closed
		// rather than panic, because this runs on a UE's own NAS goroutine and a crash
		// there would be visible to the target.
		return
	}
	class := recordClassOf(event)
	payload, err := iri.EncodeXIRI(event)
	if err != nil {
		// **A record this element could not encode is product it produced and did not
		// deliver, so it is reported.** The encoder fails on more than an internal codec
		// error: it refuses a record whose values violate the constraints its own definition
		// carries, and the commonest way to reach that is a container the network handed this
		// element that is shorter than the definition permits. A conformant mediation function would
		// discard such a record anyway; what must not happen is that neither side knows.
		//
		// NE-level and naming no target or warrant, on the channel this plane may use, and
		// off this goroutine: this runs on a UE's own NAS path.
		s.reporter.NotifyAsync(x1.NEIssueX2DeliveryLost,
			"a record could not be encoded and was not delivered")

		// **And against each warrant, naming the record type.** The NE-level report above may
		// carry neither, which is that channel's rule — so on its own it says only that
		// *something* somewhere did not encode, throttled to one per 30s, which is the same
		// signal an unreachable MDF2 produces and admits the same wrong diagnosis. An agency
		// missing one side of a policy exchange needs to know which product is absent and for
		// which warrant, and a task report is scoped to a warrant by construction.
		//
		// The live case is not an internal codec error: it is TS 33.128's own schema refusing a
		// value its prose mandates the element produce. See li/CONFORMANCE.md.
		for _, t := range tasks {
			if !t.WantsProduct(types.ProductIRI) || !produces(t, class) {
				continue
			}

			// Off this goroutine, like the element-scoped report above: this runs on a UE's
			// own NAS path, and a synchronous mTLS round trip here is a target-observable
			// delay on the subject's signalling.
			s.reporter.NotifyTaskAsync(string(t.DeliveryXID()),
				x1.TaskReportNonTerminatingFault, x1.TaskIssueRecordNotEncoded,
				fmt.Sprintf("%s: a %T record for this warrant could not be encoded and was not "+
					"delivered; the product this element produced for it is incomplete",
					x1.TaskIssueRecordNotEncoded, event))
		}

		return
	}
	for _, t := range tasks {
		if !t.WantsProduct(types.ProductIRI) || !produces(t, class) {
			continue
		}
		// A provisioned ProductID replaces the task XID in the PDU header
		// (TS 103 221-1 clause 6.2.1.2), so product is labelled with the warrant an
		// ADMF names rather than with the task carrying it.
		xid := parseXID(t.DeliveryXID())
		// The six attributes TS 33.128 table 5.3.2-2 requires, built once per task and
		// carried to every destination that task named: the sequence number belongs to
		// the (XID, Correlation ID) context, so two destinations of one task receive the
		// same number rather than two numberings of the same records.
		matched, other := t.SplitTargets(subjectIDs)
		// The number is taken here, per record *generated* for this task, and not below
		// per record delivered. That ordering is deliberate twice over: a task resolving
		// to no destination therefore consumes a number nobody receives, which is
		// harmless because nobody is receiving a stream to see a gap in (a destination
		// added mid-task sees the same effect, and is accepted for it); and moving the call
		// inside the destination loop would number each destination separately, which is
		// the per-connection numbering clause 5.3.9 forbids.
		attrs := s.ids.Attributes(xid, [x2x3.CorrelationIDLength]byte{}, at,
			types.XMLFragments(matched), types.XMLFragments(other))
		for _, addr := range s.x2Destinations(t) {
			client := s.senderFor(addr)
			if client == nil {
				continue
			}
			// Delivery is asynchronous (see Init): Send enqueues and returns, so this
			// signalling path never blocks on the MDF; delivery failures are reported
			// to the ADMF over X1 from the delivery worker, not here.
			//nolint:errcheck // async enqueue; delivery failures report via the worker, not here
			_ = client.Send(&x2x3.PDU{
				Type:          x2x3.PDUTypeX2,
				PayloadFormat: x2x3.PayloadFormat3GPP33128,
				Direction:     dir,
				XID:           xid,
				Attributes:    attrs,
				Payload:       payload,
			})
		}
	}
}

// x2Destinations is where this task's xIRI goes.
//
// The task's own destinations first, which is what TS 33.128 requires — table 6.2.2.1-1
// marks ListOfDIDs mandatory for this POI and says the endpoints "are configured using
// the CreateDestination message … prior to the task activation".
//
// **The configured MDF2 serves a task that named no destination, not one whose
// destinations produced no X2 endpoint.** Both resolve to an empty list, and they are
// different facts. A task that named nothing is a gap the provisioning function left, and
// the configured endpoint fills it, which is the case for an ADMF that provisions no
// destinations. A task that named destinations and
// yielded no X2 endpoint is an assertion this element cannot honour as stated: the live
// shape is a warrant naming an X3-only destination, where substituting the configured
// MDF2 sends an agency's signalling to an endpoint the warrant never named. On an element
// serving several agencies it is worse than a gap — the product goes to whichever
// endpoint local configuration happens to name, and isolation between agencies admits no
// exception for it.
//
// So the fallback keys on len(t.DIDs). Neither of the cases that would hit it reaches here
// through X1: a task naming an identifier this element cannot resolve is refused at
// activation with 2040, and one whose named destinations serve none of the X2 delivery its
// deliveryType orders is refused with 3040 (TS 103 221-1 clause 6.2.1.2). The test stays as
// the guard for a task arriving any other way.
func (s *subsystem) x2Destinations(t types.InterceptTask) []string {
	if addrs := t.DeliveryAddresses(types.DeliveryX2); len(addrs) > 0 {
		return addrs
	}
	if len(t.DIDs) > 0 {
		// The task named where its product goes and none of it is an X2 endpoint — refused
		// by x1 before it can be held, and refused again here. This element has nothing to
		// say about where the xIRI should go instead.
		return nil
	}
	if s.mdf2 == "" {
		return nil
	}

	return []string{s.mdf2}
}

// recordClass groups the AMF's xIRI by the per-task scoping of TS 33.128
// clause 6.2.2.2.1, under which a task may ask for the identifier-association records
// and may ask for nothing else.
type recordClass int

const (
	// classGeneral is every record outside the two below: registration, deregistration,
	// unsuccessful procedures, start of interception.
	classGeneral recordClass = iota
	classIdentifierAssociation
	classLocationUpdate
)

// recordClassOf classifies a record by its type rather than by its call site, so a new
// record type cannot be added without landing in one of the three groups.
func recordClassOf(event any) recordClass {
	switch event.(type) {
	case iri.AMFIdentifierAssociation, iri.AMFIdentifierDeassociation:
		return classIdentifierAssociation
	case iri.AMFLocationUpdate:
		return classLocationUpdate
	default:
		return classGeneral
	}
}

// produces reports whether task t is to receive a record of this class.
//
// The rule is clause 6.2.2.2.1's: the identifier-association pair is generated only for
// a task whose IdentifierAssociationExtensions asked for it, a task that asked for
// "IdentifierAssociation" gets *only* that pair and AMFLocationUpdate, and
// AMFLocationUpdate is generated under every scope.
//
// Emitted for every task, the pair would deliver records TS 33.128 says "shall not be
// generated" absent the extension to an agency that had not asked for them.
func produces(t types.InterceptTask, c recordClass) bool {
	switch c {
	case classIdentifierAssociation:
		return t.WantsIdentifierAssociation()
	case classGeneral:
		return t.WantsGeneralRecords()
	default: // classLocationUpdate, which every scope includes
		return true
	}
}

// matchingTasks returns the active tasks whose every criterion ue's identifiers satisfy,
// de-duplicated by task id. The store's index finds the tasks naming any of them; a
// task's criteria are conjunctive (TS 103 221-1 table 6.2.1.2-1), so each is then
// decided against all of ue's identifiers.
func (s *subsystem) matchingTasks(id amfctx.UeIdentity) []types.InterceptTask {
	ids := targetsOf(id)
	var out []types.InterceptTask
	seen := map[types.XID]bool{}
	for _, target := range ids {
		for _, t := range s.store.Match(target) {
			if !seen[t.XID] {
				seen[t.XID] = true
				if t.MatchedBy(ids) {
					out = append(out, t)
				}
			}
		}
	}
	return out
}

// targetsOf returns ue's known 5G target identifiers, with the AMF's "type-"
// prefixes stripped to the bare value the X1 tasking uses.
func targetsOf(id amfctx.UeIdentity) []types.TargetIdentifier {
	var ids []types.TargetIdentifier
	if v := afterPrefix(id.Supi, "imsi-", "nai-"); v != "" {
		ids = append(ids, types.TargetIdentifier{Type: types.TargetSUPI, Value: v})
	}
	if v := peiValue(id.Pei); v != "" {
		ids = append(ids, types.TargetIdentifier{Type: types.TargetPEI, Value: v})
	}
	if v := imeiOfIMEISV(id.Pei); v != "" {
		ids = append(ids, types.TargetIdentifier{Type: types.TargetPEI, Value: v})
	}
	if v := afterPrefix(id.Gpsi, "msisdn-", "extid-"); v != "" {
		ids = append(ids, types.TargetIdentifier{Type: types.TargetGPSI, Value: v})
	}
	return ids
}

// amfRegistration maps a UE identity snapshot to a TS 33.128 AMFRegistration record. The
// registration type is taken from the NAS 5GS registration request, and the result is
// the caller's (see registrationResultOf).
func amfRegistration(id amfctx.UeIdentity, result iri.AMFRegistrationResult) iri.AMFRegistration {
	return iri.AMFRegistration{
		RegistrationType:   registrationType(id),
		RegistrationResult: result,
		SUPI:               supiChoice(id),
		PEI:                peiChoice(id),
		GPSI:               gpsiChoice(id),
		GUTI:               fiveGGUTI(id),
		SUCI:               suciOf(id),
		FiveGSTAIList:      taiListOf(id),
		RATType:            ratTypeOf(id.RatType),
	}
}

// amfLocationUpdate maps a UE identity snapshot to a TS 33.128 AMFLocationUpdate record.
// The Location subtree is minimal (see li/iri.Location): it says the location is current and
// carries no cell or tracking area. The record has no member for the registration area.
func amfLocationUpdate(id amfctx.UeIdentity) iri.AMFLocationUpdate {
	return iri.AMFLocationUpdate{
		SUCI:     suciOf(id),
		SUPI:     supiChoice(id),
		PEI:      peiChoice(id),
		GPSI:     gpsiChoice(id),
		GUTI:     fiveGGUTI(id),
		Location: iri.Location{LocationInfo: iri.LocationInfo{CurrentLocation: true}},
	}
}

// amfDeregistration maps a UE identity snapshot to a TS 33.128 AMFDeregistration record, with
// the sUCI taken from identity, the mobile identity the deregistration used.
func amfDeregistration(id amfctx.UeIdentity, dir iri.AMFDirection, access iri.AccessType, identity []byte) iri.AMFDeregistration {
	return iri.AMFDeregistration{
		SUCI:                    suciFrom(identity),
		DeregistrationDirection: dir,
		AccessType:              access,
		SUPI:                    supiChoice(id),
		PEI:                     peiChoice(id),
		GPSI:                    gpsiChoice(id),
		GUTI:                    fiveGGUTI(id),
	}
}

// amfUnsuccessfulRegistration maps a rejected registration to a TS 33.128
// AMFUnsuccessfulProcedure record with a 5GMM failure cause. guti is the 5G-GUTI the
// registration used (see gutiPresented), and the record carries none when it used none:
// table 6.2.2.2.6-1's gUTI is the "5G-GUTI used in the procedure, if available".
func amfUnsuccessfulRegistration(id amfctx.UeIdentity, cause uint8, guti string) iri.AMFUnsuccessfulProcedure {
	rec := iri.AMFUnsuccessfulProcedure{
		SUCI:                suciOf(id),
		FailedProcedureType: iri.FailedRegistration,
		FailureCause:        iri.FiveGMMCause(cause),
		SUPI:                supiChoice(id),
		PEI:                 peiChoice(id),
		GPSI:                gpsiChoice(id),
	}
	if mcc, mnc, amfID, tmsi, ok := splitGUTI(guti); ok {
		rec.GUTI = iri.FiveGGUTI{MCC: iri.MCC(mcc), MNC: iri.MNC(mnc), FiveGTMSI: iri.FiveGTMSI(tmsi)}
		setAMFIdentifier(&rec.GUTI, amfID)
	}
	return rec
}

// amfStartOfInterception maps an already-registered UE identity snapshot to a TS 33.128
// AMFStartOfInterceptionWithRegisteredUE record for the access result names.
func amfStartOfInterception(id amfctx.UeIdentity, result iri.AMFRegistrationResult) iri.AMFStartOfInterceptionWithRegisteredUE {
	return iri.AMFStartOfInterceptionWithRegisteredUE{
		SUCI:               suciOf(id),
		RegistrationResult: result,
		RegistrationType:   registrationType(id),
		SUPI:               supiChoice(id),
		PEI:                peiChoice(id),
		GPSI:               gpsiChoice(id),
		GUTI:               fiveGGUTI(id),
		FiveGSTAIList:      taiListOf(id),
	}
}

// suciOf builds the sUCI record member from the octets the UE sent, or the zero
// value when the UE did not present a SUCI at all — which is every registration by
// 5G-GUTI, and so most of them. An all-zero SUCI encodes as absent, which is what
// "conditional on availability" asks for.
func suciOf(id amfctx.UeIdentity) iri.SUCI { return suciFrom(id.SuciRaw) }

// suciFrom builds the sUCI record member from a 5GS mobile identity, or the zero value when it
// is not a SUCI this element can render.
func suciFrom(identity []byte) iri.SUCI {
	suci, ok := suciFromNAS(identity)
	if !ok {
		return iri.SUCI{}
	}

	return suci
}

// taiListOf maps the UE's registration area to fiveGSTAIList. Nil when the AMF holds
// no registration area, which encodes as absent.
func taiListOf(id amfctx.UeIdentity) iri.TAIList {
	if len(id.TaiList) == 0 {
		return nil
	}
	out := make(iri.TAIList, 0, len(id.TaiList))
	for _, tai := range id.TaiList {
		tac, err := hex.DecodeString(tai.Tac)
		if err != nil || len(tac) < 2 || len(tac) > 3 {
			// TAC ::= OCTET STRING (SIZE(2..3)). A TAI we cannot render is dropped
			// rather than approximated: a wrong tracking area is a wrong location.
			continue
		}
		mcc, mnc := tai.PlmnId.GetMcc(), tai.PlmnId.GetMnc()
		if mcc == "" || mnc == "" {
			continue
		}
		out = append(out, iri.TAI{
			PLMNID: iri.PLMNID{MCC: iri.MCC(mcc), MNC: iri.MNC(mnc)},
			TAC:    tac,
		})
	}
	if len(out) == 0 {
		return nil
	}

	return out
}

// ratTypeOf maps the 5GC RAT type to TS 33.128's enumeration. An unmapped value
// yields zero, which encodes as absent — reporting no RAT type is right where
// reporting the wrong one would be a claim about the access the UE used.
func ratTypeOf(rat models.RatType) iri.RATType {
	switch rat {
	case models.RATTYPE_NR:
		return iri.RATNR
	case models.RATTYPE_EUTRA:
		return iri.RATEUTRA
	case models.RATTYPE_WLAN:
		return iri.RATWLAN
	case models.RATTYPE_VIRTUAL:
		return iri.RATVirtual
	case models.RATTYPE_NBIOT:
		return iri.RATNBIOT
	case models.RATTYPE_LTE_M:
		return iri.RATLTEM
	case models.RATTYPE_NR_U:
		return iri.RATNRU
	case models.RATTYPE_EUTRA_U:
		return iri.RATEUTRAU
	case models.RATTYPE_TRUSTED_N3_GA:
		return iri.RATTrustedN3GA
	case models.RATTYPE_TRUSTED_WLAN:
		return iri.RATTrustedWLAN
	// Non-terrestrial access — see AmfUe.IsNtn.
	case models.RATTYPE_NR_LEO:
		return iri.RATNRLEO
	case models.RATTYPE_NR_MEO:
		return iri.RATNRMEO
	case models.RATTYPE_NR_GEO:
		return iri.RATNRGEO
	case models.RATTYPE_NR_OTHER_SAT:
		return iri.RATNROtherSat
	case models.RATTYPE_NR_REDCAP:
		return iri.RATNRRedCap
	}

	return 0
}

// amfIdentifierAssociation maps a UE identity snapshot to a TS 33.128 AMFIdentifierAssociation
// record binding the target's SUPI to its assigned 5G-GUTI.
func amfIdentifierAssociation(id amfctx.UeIdentity) iri.AMFIdentifierAssociation {
	return iri.AMFIdentifierAssociation{
		SUPI:          supiChoice(id),
		PEI:           peiChoice(id),
		GPSI:          gpsiChoice(id),
		GUTI:          fiveGGUTI(id),
		SUCI:          suciOf(id),
		FiveGSTAIList: taiListOf(id),
		// Mandatory in this record: the same minimal form as AMFLocationUpdate, with no cell
		// or tracking area.
		Location: iri.Location{LocationInfo: iri.LocationInfo{CurrentLocation: true}},
	}
}

// amfIdentifierDeassociation maps a UE identity snapshot to a TS 33.128
// AMFIdentifierDeassociation record releasing the target's SUPI↔5G-GUTI binding.
func amfIdentifierDeassociation(id amfctx.UeIdentity) iri.AMFIdentifierDeassociation {
	return iri.AMFIdentifierDeassociation{
		SUPI: supiChoice(id),
		GUTI: fiveGGUTI(id),
		SUCI: suciOf(id),
		PEI:  peiChoice(id),
		GPSI: gpsiChoice(id),
	}
}

// userIdentifiers builds the TS 33.128 UserIdentifiers list the newer AMF records
// carry, from whichever identity leaves the UE context holds. It is the same
// three identities the older records carry as separate optional members; these
// records collect them into one list instead.
func userIdentifiers(id amfctx.UeIdentity) iri.UserIdentifiers {
	return iri.Identifiers(supiChoice(id), peiChoice(id), gpsiChoice(id))
}

// amfServiceAccept maps a UE identity snapshot to a TS 33.128 AMFUEServiceAccept
// record (XIRIEvent [147]).
//
// serviceMessageIdentity is the message-type octet, per TS 24.501 clause 9.7 —
// not the whole PDU. The serviceAccept arm is the one that applies: the record is
// generated because the AMF sent an accept.
//
// serviceType is the one octet of TS 24.501 clause 9.11.3.50 with the value in bits 1 to 4,
// and fiveGTMSI the 5G-TMSI of the GUTI the UE holds, omitted when it holds none.
func amfServiceAccept(id amfctx.UeIdentity, serviceType uint8) iri.AMFUEServiceAccept {
	rec := iri.AMFUEServiceAccept{
		UserIdentifiers:        userIdentifiers(id),
		ServiceMessageIdentity: iri.ServiceAcceptIdentity{nas.MsgTypeServiceAccept},
		ServiceType:            iri.ServiceType{serviceType & 0x0f},
	}
	if _, _, _, tmsi, ok := splitGUTI(id.Guti); ok {
		t := iri.FiveGTMSI(tmsi)
		rec.FiveGTMSI = &t
	}
	return rec
}

// amfUEPolicyTransfer maps a UE identity snapshot and an opaque policy container
// to a TS 33.128 AMFUEPolicyTransfer record (XIRIEvent [146]).
func amfUEPolicyTransfer(id amfctx.UeIdentity, policy []byte) iri.AMFUEPolicyTransfer {
	return iri.AMFUEPolicyTransfer{
		SUCI:     suciOf(id),
		SUPI:     supiChoice(id),
		PEI:      peiChoice(id),
		GPSI:     gpsiChoice(id),
		GUTI:     fiveGGUTI(id),
		UEPolicy: iri.UEPolicy(append([]byte(nil), policy...)),
	}
}

// AMFPositioningInfoTransfer has no reporter here on purpose. TS 33.128 clause
// 6.2.2.2.8 triggers it on positioning messages exchanged between the LMF and the
// NG-RAN *via the AMF*, and this AMF has no LMF: it decodes an uplink NRPPa PDU,
// stores the routing id and drops it. Its mandatory lcsCorrelationId is the
// TS 29.572 correlation id carried in those same LMF exchanges, not the NGAP
// routing id, so there would be nothing to populate it with either. li/iri still
// defines the record; util's li/README.md records why nothing emits it.

// supiChoice returns ue's SUPI as the iri "supi" CHOICE arm (IMSI or NAI), or
// nil when the AMF holds no SUPI in a form we can map. A nil in a mandatory SUPI
// field makes encoding fail, which deliverIRI swallows silently.
func supiChoice(id amfctx.UeIdentity) any {
	if v, ok := strings.CutPrefix(id.Supi, "imsi-"); ok {
		return iri.IMSI(v)
	}
	if v, ok := strings.CutPrefix(id.Supi, "nai-"); ok {
		return iri.NAI(v)
	}
	return nil
}

// peiChoice returns ue's PEI as the iri "pei" CHOICE arm (IMEI or IMEISV), or
// nil (an absent optional).
func peiChoice(id amfctx.UeIdentity) any {
	if v, ok := strings.CutPrefix(id.Pei, "imeisv-"); ok {
		return iri.IMEISV(v)
	}
	if v, ok := strings.CutPrefix(id.Pei, "imei-"); ok {
		return iri.IMEI(imeiWithoutCheckDigit(v))
	}
	return nil
}

// peiValue is ue's PEI in the form X1 tasking names it: the IMEISV whole, and the IMEI as
// fourteen digits.
func peiValue(pei string) string {
	if v, ok := strings.CutPrefix(pei, "imeisv-"); ok {
		return v
	}
	if v, ok := strings.CutPrefix(pei, "imei-"); ok {
		return imeiWithoutCheckDigit(v)
	}
	return ""
}

// imeiOfIMEISV is the IMEI a UE's IMEISV carries, or "" if its PEI is not an IMEISV.
//
// An IMEISV is the IMEI's Type Allocation Code and Serial Number, eight and six digits, followed
// by a two-digit Software Version Number (TS 23.003), so its first fourteen digits
// are the IMEI without its check digit: the form TS 103 221-1's peiImei names. A UE that presented
// an IMEISV is the device an IMEI warrant names, and presenting the IMEI as well is what lets that
// warrant match it. The record's PEI stays the IMEISV the UE actually presented.
func imeiOfIMEISV(pei string) string {
	v, ok := strings.CutPrefix(pei, "imeisv-")
	if !ok || len(v) != 16 {
		return ""
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return v[:14]
}

// imeiWithoutCheckDigit drops the check digit from a fifteen-digit IMEI.
//
// The AMF holds a PEI in TS 29.571 form, "imei-" and fifteen digits, because NAS carries the
// IMEI that way: TS 24.008 clause 10.5.1.4 says "the IMEI is composed of 15 digits", the last
// being the check digit TS 23.003 computes over the other fourteen. TS 33.128 defines IMEI as
// NumericString (SIZE(14)) and TS 103 221-1's peiImei as fourteen digits, so both the record
// and the target identifier X1 tasking names are the TAC and serial number alone. A value of
// any other length is passed through for the encoder to refuse, rather than guessed at.
func imeiWithoutCheckDigit(v string) string {
	if len(v) == 15 {
		return v[:14]
	}
	return v
}

// gpsiChoice returns ue's GPSI as the iri "gpsi" CHOICE arm, or nil. TS 29.571 spells a GPSI
// "msisdn-" and an MSISDN, or "extid-" and an External Identifier, which TS 23.003 clause
// 19.7.2 gives the form of a NAI; TS 33.128 carries them as GPSIMSISDN and GPSINAI. Both are
// target identifiers targetsOf matches, so a subject known only by its External Identifier is
// still named in its records.
func gpsiChoice(id amfctx.UeIdentity) any {
	if v, ok := strings.CutPrefix(id.Gpsi, "msisdn-"); ok {
		return iri.MSISDN(v)
	}
	if v, ok := strings.CutPrefix(id.Gpsi, "extid-"); ok {
		return iri.NAI(v)
	}
	return nil
}

// registrationType maps the NAS 5GS registration type to the TS 33.128 value.
func registrationType(id amfctx.UeIdentity) iri.AMFRegistrationType {
	switch id.RegistrationType5GS {
	case nasMessage.RegistrationType5GSMobilityRegistrationUpdating:
		return iri.RegTypeMobility
	case nasMessage.RegistrationType5GSPeriodicRegistrationUpdating:
		return iri.RegTypePeriodic
	case nasMessage.RegistrationType5GSEmergencyRegistration:
		return iri.RegTypeEmergency
	default:
		return iri.RegTypeInitial
	}
}

// accessType maps a 3GPP access type to the TS 33.128 enumeration.
func accessType(a models.AccessType) iri.AccessType {
	if a == models.ACCESSTYPE_NON_3_GPP_ACCESS {
		return iri.AccessNonThreeGPP
	}
	return iri.AccessThreeGPP
}

// registeredAccesses lists the accesses ue is registered on (see registeredOn), 3GPP first, as
// the registration results that name them.
func registeredAccesses(ue *amfctx.AmfUe) []iri.AMFRegistrationResult {
	var accesses []iri.AMFRegistrationResult
	for _, a := range []struct {
		access models.AccessType
		result iri.AMFRegistrationResult
	}{
		{models.ACCESSTYPE__3_GPP_ACCESS, iri.RegResult3GPPAccess},
		{models.ACCESSTYPE_NON_3_GPP_ACCESS, iri.RegResultNon3GPPAccess},
	} {
		if registeredOn(ue, a.access) {
			accesses = append(accesses, a.result)
		}
	}

	return accesses
}

// RegisteredOn reports whether ue is registered on access, as the records of this package count
// it; see registeredOn.
func RegisteredOn(ue *amfctx.AmfUe, access models.AccessType) bool { return registeredOn(ue, access) }

// registeredOn reports whether ue is registered on access: in the Registered state, or in a
// registration procedure that began from it. A UE updating its registration stays
// 5GMM-REGISTERED while the procedure runs, although the state machine has left Registered.
func registeredOn(ue *amfctx.AmfUe, access models.AccessType) bool {
	st := ue.State[access]
	if st == nil {
		return false
	}
	if st.Is(amfctx.Registered) {
		return true
	}
	return inRegistrationProcedure(ue, access) && ue.RegisteredAtProcedureStart(access)
}

// inRegistrationProcedure reports whether ue's state machine is running a registration
// procedure on access.
func inRegistrationProcedure(ue *amfctx.AmfUe, access models.AccessType) bool {
	st := ue.State[access]
	if st == nil {
		return false
	}
	switch st.Current() {
	case amfctx.Authentication, amfctx.SecurityMode, amfctx.ContextSetup:
		return true
	}
	return false
}

// taskTargets reports whether ue's identifiers satisfy every criterion task names.
func taskTargets(task types.InterceptTask, id amfctx.UeIdentity) bool {
	return task.MatchedBy(targetsOf(id))
}

// fiveGGUTI builds the 5G-GUTI the AMF actually assigned this UE.
//
// It is taken from ue's own GUTI string, which AllocateGutiToUe composes as
// PLMN ID + AmfId + 5G-TMSI, rather than from the served-GUAMI list: an AMF may
// serve several GUAMIs, and the first of them is not necessarily the one this
// UE's GUTI was cut from. Reading it back is the only way to report the identifier
// the UE is actually known by, which is the whole point of the field.
//
// The 5G-TMSI comes from the same string, as the paging identity does, and not from
// ue.Tmsi: a UE presenting a 5G-GUTI of a served GUAMI has that GUTI stored while
// ue.Tmsi keeps the value allocated earlier, and a record combining the two would name
// a 5G-GUTI the UE does not hold.
//
// The served-GUAMI list remains the fallback for a UE that has no GUTI yet. If
// even that is empty there is no PLMN to put in a GUTI, so this returns the zero
// GUTI: a record where the GUTI is optional omits it, and one where it is mandatory
// is refused by the encoder and reported, rather than sent with an empty MCC and MNC
// a conformant mediation function would discard.
func fiveGGUTI(id amfctx.UeIdentity) iri.FiveGGUTI {
	g := iri.FiveGGUTI{FiveGTMSI: iri.FiveGTMSI(uint32(id.Tmsi))}

	if mcc, mnc, amfID, tmsi, ok := splitGUTI(id.Guti); ok {
		g.MCC, g.MNC = iri.MCC(mcc), iri.MNC(mnc)
		setAMFIdentifier(&g, amfID)
		g.FiveGTMSI = iri.FiveGTMSI(tmsi)
		return g
	}

	guamis := amfctx.AMF_Self().ServedGuamiList
	if len(guamis) == 0 {
		return iri.FiveGGUTI{}
	}
	sg := guamis[0]
	g.MCC = iri.MCC(sg.PlmnId.Mcc)
	g.MNC = iri.MNC(sg.PlmnId.Mnc)
	setAMFIdentifier(&g, sg.AmfId)
	return g
}

// splitGUTI takes a 5G-GUTI apart into its PLMN, AMF identifier and 5G-TMSI components.
// The AMF composes it as MCC(3) + MNC(2..3) + AmfId(6 hex) + 5G-TMSI(8 hex), so
// the MNC's width is whatever is left once the two fixed-width tails are removed.
func splitGUTI(guti string) (mcc, mnc, amfID string, tmsi uint32, ok bool) {
	const (
		mccLen   = 3
		amfIDLen = 6
		tmsiLen  = 8
	)
	// A two- or three-digit MNC are the only valid widths.
	mncLen := len(guti) - mccLen - amfIDLen - tmsiLen
	if mncLen < 2 || mncLen > 3 {
		return "", "", "", 0, false
	}
	t, err := strconv.ParseUint(guti[len(guti)-tmsiLen:], 16, 32)
	if err != nil {
		return "", "", "", 0, false
	}
	return guti[:mccLen], guti[mccLen : mccLen+mncLen], guti[mccLen+mncLen : mccLen+mncLen+amfIDLen], uint32(t), true
}

// setAMFIdentifier unpacks the 6-hex AMF identifier, which encodes
// RegionID(8b) | SetID(10b) | Pointer(6b).
func setAMFIdentifier(g *iri.FiveGGUTI, amfID string) {
	b, err := hex.DecodeString(amfID)
	if err != nil || len(b) != 3 {
		return
	}
	v := uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
	g.AMFRegionID = iri.AMFRegionID(v >> 16 & 0xFF)
	g.AMFSetID = iri.AMFSetID(v >> 6 & 0x3FF)
	g.AMFPointer = iri.AMFPointer(v & 0x3F)
}

// parseXID converts a task's X1 identifier to the 16-byte XID carried in the
// X2 PDU header. It delegates to types.XID.Bytes so the conversion lives in one
// place across the POIs and the triggered CC-POI in the UPF.
func parseXID(xid types.XID) [16]byte {
	return xid.Bytes()
}

// afterPrefix returns s with the first matching prefix removed, or "" if none
// match (an identifier we cannot map is treated as not present).
func afterPrefix(s string, prefixes ...string) string {
	for _, p := range prefixes {
		if v, ok := strings.CutPrefix(s, p); ok {
			return v
		}
	}
	return ""
}

// keepaliveConfig turns the operator's three settings into the clause 6.2.4
// mechanism's configuration.
//
// It encodes no defaults of its own. An unset timer is passed through as zero, which
// x2x3 resolves to the specification's own value — so there is one place where "60
// seconds" is written down, and it is beside the mechanism rather than in each of the
// three network functions that run it.
//
// An unusable setting is reported to the ADMF and then discarded in favour of the
// defaults, rather than refusing to start. The alternatives are both worse: this
// subsystem is optional to the network function, so a refusal here means lawful
// interception silently does not run, and accepting the value as written can mean a
// mechanism that disconnects every delivery connection on a timer. Reporting keeps
// the operator's mistake visible to the only party that can act on it while the
// element keeps intercepting.
func keepaliveConfig(cfg Config, reporter *x1.Reporter) x2x3.KeepaliveConfig {
	ka := x2x3.KeepaliveConfig{
		Disabled: cfg.X2X3KeepaliveEnabled != nil && !*cfg.X2X3KeepaliveEnabled,
	}

	report := func(format string, args ...any) {
		if reporter != nil {
			reporter.NotifyAsync(x1.NEIssueInvalidConfig, fmt.Sprintf(format, args...))
		}
	}

	for _, t := range []struct {
		name  string
		value string
		into  *time.Duration
	}{
		{"x2x3KeepaliveTimeP1", cfg.X2X3KeepaliveTimeP1, &ka.TimeP1},
		{"x2x3KeepaliveTimeP2", cfg.X2X3KeepaliveTimeP2, &ka.TimeP2},
	} {
		if t.value == "" {
			continue
		}
		d, err := time.ParseDuration(t.value)
		if err != nil {
			report("%s is not a duration; the specification's default is used instead", t.name)

			continue
		}
		*t.into = d
	}

	if err := ka.Validate(); err != nil {
		report("the configured X2/X3 keepalive timers are unusable and the specification's "+
			"defaults are used instead: %v", err)

		return x2x3.KeepaliveConfig{Disabled: ka.Disabled}
	}

	return ka
}

// configuredDestinations maps the pre-shared destinations from configuration onto the
// form the X1 server resolves them in, and tells the ADMF about any it cannot use.
//
// An entry that does not resolve is not a small mistake: unless the ADMF provisions that DID
// itself, a task naming it is refused with 2040 (DID does not exist on the NE), so an
// operator's typo stops the warrant it was meant for. There is no general log to say so on — this plane deliberately writes to none —
// and the count is all the ADMF needs, since the entries themselves are the operator's
// configuration and not the ADMF's business.
func configuredDestinations(dests []Destination, reporter *x1.Reporter) []x1.ConfiguredDestination {
	out := make([]x1.ConfiguredDestination, 0, len(dests))
	var rejected int
	for _, d := range dests {
		entry := x1.ConfiguredDestination{DID: d.DID, DeliveryType: d.DeliveryType, Address: d.Address}
		if entry.Valid() != nil {
			rejected++

			continue
		}
		out = append(out, entry)
	}
	if rejected > 0 {
		reporter.NotifyAsync(x1.NEIssueInvalidConfig, fmt.Sprintf(
			"%d configured delivery destination(s) are unusable and were dropped; "+
				"a task naming one will be refused", rejected))
	}

	return out
}

// resolvableTargets are the identifier kinds this element can match a subject on.
// It is targetsOf's counterpart: what that function can produce is exactly what a
// warrant must name for this element to be able to act on it.
var resolvableTargets = []types.TargetIdentifierType{
	types.TargetSUPI, types.TargetPEI, types.TargetGPSI,
}

// canApply refuses tasking this element cannot act on, before it is acknowledged.
//
// **Both halves of "cannot act on": the identifiers and the products.** This element is an
// IRI-POI and produces xIRI, so a warrant requiring only content is one it can never make
// anything for. Acknowledged, an `X3Only` task naming a valid SUPI would be stored, reported
// as active in an interrogation, and skipped by every delivery path for not requesting IRI:
// the provisioning function is told an interception is running at an element that can never
// produce anything for it, which is indistinguishable from the outside from a tasked subject
// who did nothing. The requirement is written in terms of criteria and the CC-POI applies the
// product half; this is the same test on the other side.
//
// `X2andX3` is accepted, and that distinction matters: a warrant provisioned to several
// elements legitimately names products only some of them produce, and refusing it here would
// refuse the ordinary multi-element warrant. What is refused is a task requiring *none* of
// what this element makes.
func canApply(task types.InterceptTask) error {
	if len(task.Targets) == 0 {
		return errors.New("li: task names no target identifiers")
	}
	// Every criterion must match (TS 103 221-1 table 6.2.1.2-1), so one this element
	// never evaluates makes the task unmatchable here, and so does a combination no
	// subscriber presents; the same table has the element reject both rather than
	// acknowledge an interception that can never run.
	if !task.NamesOnlyTypes(resolvableTargets...) {
		return errors.New("li: task names an identifier this element does not resolve, and " +
			"every criterion of a task must match; it matches subjects by SUPI, PEI or GPSI")
	}
	if a, b, ok := task.ContradictoryTargets(); ok {
		return fmt.Errorf("li: task names %s and %s, which no subscriber presents together, "+
			"and every criterion of a task must match", a.Type, b.Type)
	}
	if !task.WantsProduct(types.ProductIRI) {
		return errors.New("li: task requires no product this element produces; " +
			"an AMF IRI-POI produces xIRI (X2), so a task whose deliveryType is X3Only can " +
			"never yield anything here")
	}
	// **And the third axis: destinations that cannot carry the product.** A task can pass both
	// tests above — its identifiers resolve, its delivery type asks for IRI — and still be one
	// this element can produce nothing for, because the destinations it names resolve to no X2
	// endpoint. The live shape is an ADMF that provisioned a destination for the warrant's CC
	// leg and then named it on the IRI task too.
	//
	// Acknowledged, it is stored, reported as `provisioningStatus: complete` with an empty fault
	// list, and consumes a sequence number per record while delivering to zero addresses. Every
	// account this element gives of itself says the interception is running, which is the exact
	// condition the two checks above exist to prevent, reached by the one route they do not
	// cover. x2Destinations already declines to substitute the configured MDF2 here — correctly,
	// since that would send one agency's product to an endpoint the warrant never named — so
	// refusing the task is what closes the gap rather than widening it.
	//
	// Keyed on len(task.DIDs), exactly as x2Destinations is: a task naming *no* destination is a
	// gap the provisioning function left and the configured endpoint fills it, which is a
	// different fact and stays accepted here; canApplyHere refuses it when no MDF2 is configured.
	if len(task.DIDs) > 0 && len(task.DeliveryAddresses(types.DeliveryX2)) == 0 {
		return errors.New("li: task names delivery destinations but none of them is an X2 " +
			"endpoint; an AMF IRI-POI delivers xIRI over X2, so every record this task " +
			"produced would be built, numbered and delivered to nowhere")
	}

	return nil
}

// canApplyHere is canApply and the one test that needs this element's configuration: a task
// naming no destination is served by the configured MDF2 (see x2Destinations), so with no MDF2
// configured it is served by nothing. Acknowledged, it would be held and reported running while
// every record it produced was numbered and delivered nowhere, the condition canApply refuses
// for a task whose named destinations give no X2 endpoint. MDF2 itself stays optional: an ADMF
// that names a destination on every task needs none.
func (s *subsystem) canApplyHere(task types.InterceptTask) error {
	if err := canApply(task); err != nil {
		return err
	}
	if len(task.DIDs) == 0 && s.mdf2 == "" {
		return errors.New("li: task names no delivery destination and this element has no " +
			"configured X2 endpoint to serve one, so every record this task produced would be " +
			"delivered to nowhere")
	}

	return nil
}

// keepaliveProcedure reads the X1 keepalive procedure an operator configured, or nil when
// they disabled it (TS 103 221-1 clause 6.6.2: "By default (with no prior agreement) they
// are enabled", so only a stated agreement turns it off).
func keepaliveProcedure(cfg Config) (*x1.KeepaliveProcedure, error) {
	if cfg.KeepaliveDisabled {
		return nil, nil //nolint:nilnil // a disabled procedure is not an error
	}

	p := x1.KeepaliveProcedure{RetainTasking: cfg.KeepaliveRetainTasking}
	for _, t := range []struct {
		name, value string
		into        *time.Duration
	}{
		{"keepaliveTimeout", cfg.KeepaliveTimeout, &p.TimeP2},
		{"keepaliveTimeP1", cfg.KeepaliveTimeP1, &p.TimeP1},
		{"keepaliveTimeP3", cfg.KeepaliveTimeP3, &p.TimeP3},
	} {
		d, err := parseKeepaliveTimer(t.name, t.value)
		if err != nil {
			return nil, err
		}
		*t.into = d
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}

	return &p, nil
}

// parseKeepaliveTimer reads one keepalive timer an operator wrote. Empty is the clause
// default, returned as zero.
//
// A value that does not parse is refused rather than read as zero: zero means "the
// default", so discarding the parse error would turn a mistyped duration into a silently
// different timer on an element that otherwise looks healthy.
//
// A non-positive duration is refused for the same reason: "0s" and "-5m" are values an
// operator wrote, and neither can mean the timer they asked for. So is one below
// x1.MinKeepaliveWindow, which the procedure cannot measure; the library refuses it too,
// for any caller that does not check.
func parseKeepaliveTimer(name, v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("li: %s %q is not a duration", name, v)
	}
	if d <= 0 {
		return 0, fmt.Errorf("li: %s %q is not a positive duration", name, v)
	}
	if d < x1.MinKeepaliveWindow {
		return 0, fmt.Errorf("li: %s %q is shorter than the minimum keepalive timer %s", name, v, x1.MinKeepaliveWindow)
	}

	return d, nil
}
