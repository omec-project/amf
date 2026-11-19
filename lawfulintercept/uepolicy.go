// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"sync"
	"time"
	"weak"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/util/li/iri"
)

// The UE policy delivery service message types this package reads, TS 24.501 table D.6.1.1.
// A UE policy container holds one such message: the PTI in its first octet and the message
// type in its second (tables D.5.1.1.1 and D.5.2.1.1).
const (
	uePolicyCommand       = 0x01 // MANAGE UE POLICY COMMAND
	uePolicyComplete      = 0x02 // MANAGE UE POLICY COMPLETE
	uePolicyCommandReject = 0x03 // MANAGE UE POLICY COMMAND REJECT
)

// Bounds on the COMMANDs held while their answers are awaited. A procedure is normally one
// PTI at a time, retransmitted under the same PTI on T3501, so a UE rarely holds more than
// one. Past maxPendingUEs live UEs, the UE whose newest COMMAND is oldest is dropped to make
// room, so UEs that leave without answering cannot keep a later target from being recorded.
const (
	maxPendingPerUE   = 4
	maxPendingUEs     = 1024
	uePolicyHeaderLen = 2
)

// uePolicyHoldFor is how long a COMMAND is held for its answer. The PCF supervises the
// procedure with T3501, retransmitting the COMMAND under the same PTI on each of its first
// four expiries and abandoning the procedure on the fifth (TS 24.501 clause D.2.1.5 a). Each
// retransmission this AMF relays replaces the COMMAND held under its PTI and restarts its
// hold, so the hold only has to outlast one T3501 period plus the time the answer takes to
// come back. T3501's value is network dependent (table D.7.1, NOTE 1) and this element cannot
// read the PCF's; six minutes is a generous bound on that, not a multiple of it.
const uePolicyHoldFor = 6 * time.Minute

// pendingCommand is a COMMAND relayed to a UE and not yet answered.
type pendingCommand struct {
	at      time.Time // when it was relayed
	command []byte
	pti     uint8
}

// pendingPolicies holds, per UE, the MANAGE UE POLICY COMMANDs this AMF relayed to it, so the
// AMFUEPolicyTransfer the UE's COMPLETE triggers can carry the COMMAND it confirms. The zero
// value is ready to use.
//
// It is keyed by a weak pointer, so an entry never keeps a UE context alive. A COMPLETE does not
// end an entry: a UE that receives a retransmitted COMMAND it has already answered retransmits
// its COMPLETE (TS 24.501 clause D.2.1.6 e), and that COMPLETE confirms the same COMMAND. An
// entry ends when a COMMAND under the same PTI replaces it, on a COMMAND REJECT, when the
// bounds above evict it, and when the UE context is removed (amfctx.SetRemovalHook) or the UE
// is registered on no access. Its hold expiring stops it confirming anything at once, but it
// is pruned — and stops counting toward maxPendingUEs — only on the next remember, as is the
// entry of a UE context that has been collected.
type pendingPolicies struct {
	byUE map[weak.Pointer[amfctx.AmfUe]][]pendingCommand
	now  func() time.Time // nil means time.Now; a test substitutes a clock
	mu   sync.Mutex
}

func (p *pendingPolicies) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// expireLocked drops every COMMAND whose hold has run out, and every entry whose UE context
// has been collected, so neither counts toward maxPendingUEs and pushes a live UE's COMMAND
// out. At most maxPendingUEs × maxPendingPerUE entries, on a path taken once per downlink
// policy container to a target.
func (p *pendingPolicies) expireLocked(now time.Time) {
	for ue, held := range p.byUE {
		if ue.Value() == nil {
			delete(p.byUE, ue)
			continue
		}
		kept := held[:0]
		for _, c := range held {
			if now.Sub(c.at) < uePolicyHoldFor {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			delete(p.byUE, ue)
		} else {
			p.byUE[ue] = kept
		}
	}
}

// evictOldestLocked drops the UE whose most recent COMMAND is the oldest.
func (p *pendingPolicies) evictOldestLocked() {
	var (
		oldest weak.Pointer[amfctx.AmfUe]
		when   time.Time
		found  bool
	)
	for ue, held := range p.byUE {
		latest := held[len(held)-1].at
		if !found || latest.Before(when) {
			oldest, when, found = ue, latest, true
		}
	}
	if found {
		delete(p.byUE, oldest)
	}
}

// remember holds command for ue under pti, replacing a COMMAND held under the same PTI — a
// T3501 retransmission, or a new procedure reusing the value.
func (p *pendingPolicies) remember(ue *amfctx.AmfUe, pti uint8, command []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.clock()
	if p.byUE == nil {
		p.byUE = map[weak.Pointer[amfctx.AmfUe]][]pendingCommand{}
	}
	p.expireLocked(now)

	key := weak.Make(ue)
	held, known := p.byUE[key]
	if !known && len(p.byUE) >= maxPendingUEs {
		p.evictOldestLocked()
	}
	for i := range held {
		if held[i].pti == pti {
			held = append(held[:i], held[i+1:]...)
			break
		}
	}
	if len(held) >= maxPendingPerUE {
		held = held[1:]
	}
	p.byUE[key] = append(held, pendingCommand{pti: pti, command: append([]byte(nil), command...), at: now})
}

// lookup returns the COMMAND held for ue under pti, or nil when none is held or its hold has
// run out. The entry stays: a retransmitted COMPLETE confirms the same COMMAND.
func (p *pendingPolicies) lookup(ue *amfctx.AmfUe, pti uint8) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, c := range p.byUE[weak.Make(ue)] {
		if c.pti != pti {
			continue
		}
		if p.clock().Sub(c.at) >= uePolicyHoldFor {
			return nil
		}
		return c.command
	}
	return nil
}

// drop removes the COMMAND held for ue under pti, if any.
func (p *pendingPolicies) drop(ue *amfctx.AmfUe, pti uint8) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := weak.Make(ue)
	held := p.byUE[key]
	for i := range held {
		if held[i].pti != pti {
			continue
		}
		held = append(held[:i], held[i+1:]...)
		if len(held) == 0 {
			delete(p.byUE, key)
		} else {
			p.byUE[key] = held
		}
		return
	}
}

// forget drops everything held for ue.
func (p *pendingPolicies) forget(ue *amfctx.AmfUe) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.byUE, weak.Make(ue))
}

// held reports how many UEs have a COMMAND held, for tests.
func (p *pendingPolicies) held() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.byUE)
}

// Registered for the life of the process rather than in Init: it does nothing while LI is
// inactive, and no path that installs a subsystem can then forget it.
func init() { amfctx.SetRemovalHook(forgetRemovedUE) }

// forgetRemovedUE is the UE-context removal hook: a removed context's procedures will not
// complete through it.
func forgetRemovedUE(ue *amfctx.AmfUe) {
	if sub := active.Load(); sub != nil {
		sub.policies.forget(ue)
	}
}

// NoteUEPolicyCommand is the downlink half of the UE policy record: the AMF relaying a UE
// policy container to the UE. It produces no record. TS 33.128 clause 6.2.2.2.12 triggers
// AMFUEPolicyTransfer on the UE's MANAGE UE POLICY COMPLETE, and makes its uEPolicy the
// MANAGE UE POLICY COMMAND that COMPLETE confirms, so a COMMAND relayed to a UE under a
// warrant is held until its answer arrives.
//
// Only a UE with a matching task that is to receive the record is held for, so a subscriber
// under no warrant costs nothing here, and neither does one whose warrants are scoped to
// identifier association or take no IRI. The consequence is that a task activated between a
// COMMAND and its COMPLETE does not report that exchange: the COMPLETE finds nothing held.
//
// No-op and silent when LI is inactive or ue is not a target.
func NoteUEPolicyCommand(ue *amfctx.AmfUe, container []byte) {
	sub := active.Load()
	if sub == nil || ue == nil || len(container) < uePolicyHeaderLen || container[1] != uePolicyCommand {
		return
	}
	if !sub.anyTaskReceives(ue.IdentitySnapshot(), iri.AMFUEPolicyTransfer{}) {
		return
	}
	sub.policies.remember(ue, container[0], container)
}

// ReportUEPolicyComplete is the uplink half: the AMF passing a UE policy container from the UE
// to the PCF in Namf_Communication_N1MessageNotify. A MANAGE UE POLICY COMPLETE produces one
// AMFUEPolicyTransfer carrying the COMMAND held under its PTI. A retransmitted COMPLETE (TS
// 24.501 clause D.2.1.6 e) produces another: TS 33.128 clause 6.2.2.2.12 triggers on each
// N1MessageNotify carrying a COMPLETE, and the COMMAND stays held for it.
//
// A COMPLETE with nothing held produces none, because the record's uEPolicy is mandatory and
// the COMPLETE itself is not that content; it is reported to each matching warrant as
// recordNotProduced instead. Nothing is held when the COMMAND was relayed before the UE was a
// target or before this AMF started, when its hold ran out or the bounds above evicted it,
// after a COMMAND REJECT under the same PTI, or when the UE answered under a PTI other than the
// COMMAND's, which TS 24.501 clause D.1.2 allows of earlier UEs. A MANAGE UE POLICY COMMAND
// REJECT ends the procedure without a record, since the clause names only the COMPLETE. The
// policy is copied, never parsed beyond those two octets.
//
// No-op and silent when LI is inactive or ue is not a target.
func ReportUEPolicyComplete(ue *amfctx.AmfUe, container []byte) {
	sub := active.Load()
	if sub == nil || ue == nil || len(container) < uePolicyHeaderLen {
		return
	}
	switch container[1] {
	case uePolicyComplete:
		command := sub.policies.lookup(ue, container[0])
		if command == nil {
			// The mandated record cannot be produced, and this is the one place that knows it,
			// so the loss is reported to each warrant that wanted it rather than left silent.
			sub.reportNotProduced(ue.IdentitySnapshot(), iri.AMFUEPolicyTransfer{},
				"a UE policy COMPLETE arrived with no COMMAND held for its PTI")
			return
		}
		id := ue.IdentitySnapshot()
		sub.reportEvent(id, amfUEPolicyTransfer(id, command))
	case uePolicyCommandReject:
		sub.policies.drop(ue, container[0])
	}
}
