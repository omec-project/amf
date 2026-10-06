// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package producer

import (
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
)

func HandleCreateAMFEventSubscription(request *httpwrapper.Request) *httpwrapper.Response {
	createEventSubscription := request.Body.(models.AmfCreateEventSubscription)

	createdEventSubscription, problemDetails := CreateAMFEventSubscriptionProcedure(createEventSubscription)
	if createdEventSubscription != nil {
		return httpwrapper.NewResponse(http.StatusCreated, nil, createdEventSubscription)
	} else if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	} else {
		problemDetails := utils.ProblemDetailsWithCause("Unspecified NF failure", http.StatusInternalServerError, "Unspecified NF failure", utils.CauseUnspecifiedNfFailure)
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
}

// TODO: handle event filter
func CreateAMFEventSubscriptionProcedure(createEventSubscription models.AmfCreateEventSubscription) (
	*models.AmfCreatedEventSubscription, *models.ProblemDetails,
) {
	amfSelf := context.AMF_Self()

	subscription := createEventSubscription.GetSubscription()

	if reflect.DeepEqual(subscription, models.AmfEventSubscription{}) {
		problemDetails := utils.ProblemDetailsWithCause("Subscription empty", http.StatusBadRequest, "Event subscription is empty", utils.CauseSubscriptionEmpty)
		return nil, problemDetails
	}

	contextEventSubscription := context.AMFContextEventSubscription{}
	contextEventSubscription.EventSubscription = subscription
	var isImmediate bool
	var immediateFlags []bool
	var reportlist []models.AmfEventReport

	id, err := amfSelf.EventSubscriptionIDGenerator.Allocate()
	if err != nil {
		problemDetails := utils.ProblemDetailsWithCause("Unspecified NF failure", http.StatusInternalServerError, "Failed to allocate subscription ID", utils.CauseUnspecifiedNfFailure)
		return nil, problemDetails
	}
	newSubscriptionID := strconv.Itoa(int(id))

	// store subscription in context
	ueEventSubscription := context.AmfUeEventSubscription{}
	// The UE's copy gets a list of its own. NewExtAmfEventSubscription stores the slice it is
	// handed, so passing the subscription's own list would leave the two sharing one array -- and
	// a patch of that subscription writes into it: "replace" assigns an element in place, "remove"
	// shifts the elements down. A UE's list would change underneath whoever is reading it, with
	// nothing ordering the two, and be persisted half-patched.
	//
	// It is the list that is copied, not what each event points at. That is the depth the patching
	// works at: it replaces and moves whole events, and never reaches inside one.
	subscribedEvents := contextEventSubscription.EventSubscription.GetEventList()
	ueEvents := make([]models.AmfEvent, len(subscribedEvents))
	copy(ueEvents, subscribedEvents)

	extAmfEventSubscription := models.NewExtAmfEventSubscription(ueEvents, contextEventSubscription.EventSubscription.GetEventNotifyUri(), contextEventSubscription.EventSubscription.GetNotifyCorrelationId(), contextEventSubscription.EventSubscription.GetNfId())
	// NewExtAmfEventSubscription only sets the required fields. Options carries the subscription's
	// trigger/expiry/maxReports, which NewAmfEventReport reads to compute each report's state, so it
	// must be propagated explicitly; the remaining fields are optional and unused by the AMF.
	if subscription.HasOptions() {
		options := subscription.GetOptions()
		if options.HasMaxReports() {
			maxReports := options.GetMaxReports()
			options.SetMaxReports(maxReports)
		}
		extAmfEventSubscription.SetOptions(options)
	}
	ueEventSubscription.EventSubscription = extAmfEventSubscription
	ueEventSubscription.Timestamp = time.Now().UTC()

	if subscription.HasOptions() && subscription.GetOptions().Trigger == models.AMFEVENTTRIGGER_CONTINUOUS {
		opts := subscription.GetOptions()
		if opts.HasMaxReports() {
			remaining := opts.GetMaxReports()
			ueEventSubscription.RemainReports = &remaining
		}
	}

	if subscription.EventList == nil {
		problemDetails := utils.ProblemDetailsWithCause("Event list empty", http.StatusBadRequest, "Event list is empty", utils.CauseSubscriptionEventlistEmpty)
		return nil, problemDetails
	}

	for _, events := range subscription.EventList {
		immediateFlags = append(immediateFlags, events.GetImmediateFlag())
		if events.GetImmediateFlag() {
			isImmediate = true
		}
	}

	if subscription.GetAnyUE() {
		contextEventSubscription.IsAnyUe = true
		ueEventSubscription.AnyUe = true
		amfSelf.UePool.Range(func(key, value any) bool {
			ue := value.(*context.AmfUe)
			ueSubscription := ueEventSubscription
			ue.SetEventSubscription(newSubscriptionID, &ueSubscription)
			contextEventSubscription.UeSupiList = append(contextEventSubscription.UeSupiList, ue.GetSupi())
			return true
		})
	} else if subscription.GetGroupId() != "" {
		contextEventSubscription.IsGroupUe = true
		ueEventSubscription.AnyUe = true
		amfSelf.UePool.Range(func(key, value any) bool {
			ue := value.(*context.AmfUe)
			if ue.GroupID == subscription.GetGroupId() {
				ueSubscription := ueEventSubscription
				ue.SetEventSubscription(newSubscriptionID, &ueSubscription)
				contextEventSubscription.UeSupiList = append(contextEventSubscription.UeSupiList, ue.GetSupi())
			}
			return true
		})
	} else {
		if ue, ok := amfSelf.AmfUeFindBySupi(subscription.GetSupi()); !ok {
			problemDetails := utils.ProblemDetailsWithCause("UE not served by AMF", http.StatusForbidden, "UE is not served by this AMF", utils.CauseUeNotServedByAmf)
			return nil, problemDetails
		} else {
			ueSubscription := ueEventSubscription
			ue.SetEventSubscription(newSubscriptionID, &ueSubscription)
			contextEventSubscription.UeSupiList = append(contextEventSubscription.UeSupiList, ue.GetSupi())
		}
	}

	// delete subscription
	if subscription.HasOptions() {
		contextEventSubscription.Expiry = subscription.GetOptions().Expiry
	}
	// Snapshot the subscription for the response before publishing it: once it is in the map a
	// concurrent PATCH on the (predictable) ID could mutate the shared EventList/Options while this
	// response is being serialized. The copy gives the response its own event list and options.
	responseSubscription := cloneEventSubscriptionForResponse(subscription)
	amfSelf.NewEventSubscription(newSubscriptionID, &contextEventSubscription)

	// build response
	createdEventSubscription := models.NewAmfCreatedEventSubscription(responseSubscription, newSubscriptionID)

	// for immediate use
	if subscription.GetAnyUE() {
		amfSelf.UePool.Range(func(key, value any) bool {
			ue := value.(*context.AmfUe)
			if isImmediate {
				subReports(ue, newSubscriptionID)
			}
			for i, flag := range immediateFlags {
				if flag {
					report, ok := NewAmfEventReport(ue, subscription.EventList[i].Type, newSubscriptionID)
					if ok {
						reportlist = append(reportlist, report)
					}
				}
			}
			// delete subscription
			if reportlistLen := len(reportlist); reportlistLen > 0 && (!reportlist[reportlistLen-1].State.Active) {
				ue.DeleteEventSubscription(newSubscriptionID)
			}
			return true
		})
	} else if subscription.GetGroupId() != "" {
		amfSelf.UePool.Range(func(key, value any) bool {
			ue := value.(*context.AmfUe)
			if isImmediate {
				subReports(ue, newSubscriptionID)
			}
			if ue.GroupID == subscription.GetGroupId() {
				for i, flag := range immediateFlags {
					if flag {
						report, ok := NewAmfEventReport(ue, subscription.EventList[i].Type, newSubscriptionID)
						if ok {
							reportlist = append(reportlist, report)
						}
					}
				}
				// delete subscription
				if reportlistLen := len(reportlist); reportlistLen > 0 && (!reportlist[reportlistLen-1].State.Active) {
					ue.DeleteEventSubscription(newSubscriptionID)
				}
			}
			return true
		})
	} else {
		ue, _ := amfSelf.AmfUeFindBySupi(subscription.GetSupi())
		if isImmediate {
			subReports(ue, newSubscriptionID)
		}
		for i, flag := range immediateFlags {
			if flag {
				report, ok := NewAmfEventReport(ue, subscription.EventList[i].Type, newSubscriptionID)
				if ok {
					reportlist = append(reportlist, report)
				}
			}
		}
		// delete subscription
		if reportlistLen := len(reportlist); reportlistLen > 0 && (!reportlist[reportlistLen-1].State.Active) {
			ue.DeleteEventSubscription(newSubscriptionID)
		}
	}
	if len(reportlist) > 0 {
		createdEventSubscription.ReportList = reportlist
		// delete subscription
		if !reportlist[0].State.GetActive() {
			// Route removal through the lock-and-identity-checked path instead of freeing the ID
			// directly: a bare context delete here would bypass the subscription mutex, so it could
			// free the ID for reuse while a concurrent modify or delete holds the lock, and the
			// in-flight operation would then corrupt the replacement subscription's entries.
			_ = DeleteAMFEventSubscriptionProcedure(newSubscriptionID)
		}
	}

	return createdEventSubscription, nil
}

func HandleDeleteAMFEventSubscription(request *httpwrapper.Request) *httpwrapper.Response {
	logger.EeLog.Infoln("Handle Delete AMF Event Subscription")

	subscriptionID := request.Params["subscriptionId"]

	problemDetails := DeleteAMFEventSubscriptionProcedure(subscriptionID)
	if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	} else {
		return httpwrapper.NewResponse(http.StatusOK, nil, nil)
	}
}

func DeleteAMFEventSubscriptionProcedure(subscriptionID string) *models.ProblemDetails {
	amfSelf := context.AMF_Self()

	subscription, ok := amfSelf.FindEventSubscription(subscriptionID)
	if !ok {
		problemDetails := utils.ProblemDetailsWithCause("Subscription not found", http.StatusNotFound, "Event subscription not found", utils.CauseSubscriptionNotFound)
		return problemDetails
	}

	// Serialize with ModifyAMFEventSubscriptionProcedure on the same object: it holds this mutex
	// across its per-UE writes, so taking it here keeps the delete -- which frees the subscription
	// ID for reuse -- from interleaving mid-modify and redirecting a PATCH to a reused ID. After
	// locking, confirm the map still points at this object; a concurrent delete may already have
	// removed it, and freeing an ID that a create has since reused would corrupt the new entry.
	subscription.Mutex.Lock()
	defer subscription.Mutex.Unlock()

	if current, found := amfSelf.FindEventSubscription(subscriptionID); !found || current != subscription {
		problemDetails := utils.ProblemDetailsWithCause("Subscription not found", http.StatusNotFound, "Event subscription not found", utils.CauseSubscriptionNotFound)
		return problemDetails
	}

	for _, supi := range subscription.UeSupiList {
		if ue, ok := amfSelf.AmfUeFindBySupi(supi); ok {
			ue.DeleteEventSubscription(subscriptionID)
		}
	}
	amfSelf.DeleteEventSubscription(subscriptionID)
	return nil
}

func HandleModifyAMFEventSubscription(request *httpwrapper.Request) *httpwrapper.Response {
	logger.EeLog.Infoln("Handle Modify AMF Event Subscription")

	subscriptionID := request.Params["subscriptionId"]
	modifySubscriptionRequest := request.Body.(models.ModifySubscriptionRequest)

	updatedEventSubscription, problemDetails := ModifyAMFEventSubscriptionProcedure(subscriptionID,
		modifySubscriptionRequest)
	if updatedEventSubscription != nil {
		return httpwrapper.NewResponse(http.StatusOK, nil, updatedEventSubscription)
	} else if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	} else {
		problemDetails = utils.ProblemDetailsWithCause("Unspecified NF failure", http.StatusInternalServerError, "Unspecified NF failure", utils.CauseUnspecifiedNfFailure)
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
}

// modifyAfterLookupHook runs, when set, between locating the subscription and acquiring its lock in
// ModifyAMFEventSubscriptionProcedure. It is a test seam for deterministically interposing a
// delete/recreate that reuses the subscription ID, exercising the post-lock identity revalidation.
// It is nil in production and carries no cost beyond a nil check.
var modifyAfterLookupHook func()

func ModifyAMFEventSubscriptionProcedure(
	subscriptionID string,
	modifySubscriptionRequest models.ModifySubscriptionRequest) (
	*models.AmfUpdatedEventSubscription, *models.ProblemDetails,
) {
	amfSelf := context.AMF_Self()

	contextSubscription, ok := amfSelf.FindEventSubscription(subscriptionID)
	if !ok {
		problemDetails := utils.ProblemDetailsWithCause("Subscription not found", http.StatusNotFound, "Event subscription not found", utils.CauseSubscriptionNotFound)
		return nil, problemDetails
	}

	if modifyAfterLookupHook != nil {
		modifyAfterLookupHook()
	}

	// The subscription is a single object shared through a sync.Map that has no lock of its own, so
	// guard both patch paths and the response snapshot below: concurrent modify requests otherwise
	// race on its options and event list, and a later modify races the serialization of a response
	// returned to an earlier one. The expiry path nests ue.Mutex under this lock when it propagates
	// to each UE; no path takes them in the opposite order, so the ordering is deadlock-free.
	contextSubscription.Mutex.Lock()
	defer contextSubscription.Mutex.Unlock()

	// A delete can remove this subscription -- freeing its ID for reuse by a later create -- between
	// the lookup above and acquiring the lock. Confirm the map still points at the object we locked
	// before touching any UE state; otherwise a reused ID would misdirect this PATCH to a different
	// subscription's per-UE entries. Delete takes the same mutex, so once this check passes the
	// object cannot be removed while the lock is held.
	if current, found := amfSelf.FindEventSubscription(subscriptionID); !found || current != contextSubscription {
		problemDetails := utils.ProblemDetailsWithCause("Subscription not found", http.StatusNotFound, "Event subscription not found", utils.CauseSubscriptionNotFound)
		return nil, problemDetails
	}

	if modifySubscriptionRequest.ArrayOfAmfUpdateEventOptionItem != nil {
		optionItems := *modifySubscriptionRequest.ArrayOfAmfUpdateEventOptionItem
		// An empty patch list changes nothing; reject it rather than acknowledge a no-op, and so the
		// indexing below is always safe.
		if len(optionItems) == 0 {
			problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Option patch list is empty")
			return nil, problemDetails
		}
		// Validate every item before touching state. The AMF acts only on the expiry; the other two
		// paths the v2.2.5 model defines (notifFlag, mutingExcInstructions) are schema-valid but the
		// AMF does not implement muting, so they are rejected as a capability gap (501) rather than as
		// malformed input (400). Anything outside the model's path set, or a non-replace op on expiry,
		// is genuinely malformed.
		const (
			expiryPath    = "/options/expiry"
			notifFlagPath = "/options/notifFlag"
			mutingExcPath = "/options/mutingExcInstructions"
		)
		for _, item := range optionItems {
			switch item.GetPath() {
			case expiryPath:
				if item.GetOp() != "replace" {
					problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Unsupported operation for " + expiryPath + "; only replace is supported")
					return nil, problemDetails
				}
			case notifFlagPath, mutingExcPath:
				problemDetails := utils.ProblemDetailsNotImplemented("Modifying " + item.GetPath() + " is not supported by the AMF")
				return nil, problemDetails
			default:
				problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Unsupported option patch path " + item.GetPath())
				return nil, problemDetails
			}
		}
		// A /options/expiry patch targets the subscription's options block, which is optional and may
		// be absent. Reject the patch before mutating anything rather than acknowledging an update
		// that cannot take effect: without options the canonical response carries no expiry and
		// SetEventSubscriptionExpiry no-ops for every UE, so a success response would be a lie.
		options, ok := contextSubscription.EventSubscription.GetOptionsOk()
		if !ok || options == nil {
			problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Subscription has no options to modify")
			return nil, problemDetails
		}
		// RFC 6902 applies patch operations in order; every item here replaces the same expiry field,
		// so the final value is the last item's -- equivalent to applying each in turn.
		expiry0 := optionItems[len(optionItems)-1].GetValue()
		contextSubscription.Expiry = &expiry0
		// Reflect the new expiry where it is actually read: contextSubscription.Expiry alone is on
		// no read path. The canonical subscription's options feed the response returned below, and
		// NewAmfEventReport reads expiry from each UE's own options, so both must be updated or a
		// later report would use the original expiry and return the wrong active state.
		options.SetExpiry(expiry0)
		for _, supi := range contextSubscription.UeSupiList {
			expiry := expiry0
			if ue, ok := amfSelf.AmfUeFindBySupi(supi); ok {
				ue.SetEventSubscriptionExpiry(subscriptionID, expiry)
			}
		}
	} else if modifySubscriptionRequest.ArrayOfAmfUpdateEventSubscriptionItem != nil {
		subscription := &contextSubscription.EventSubscription
		if !contextSubscription.IsAnyUe && !contextSubscription.IsGroupUe {
			if _, ok := amfSelf.AmfUeFindBySupi(subscription.GetSupi()); !ok {
				problemDetails := utils.ProblemDetailsWithCause("UE not served by AMF", http.StatusForbidden, "UE is not served by this AMF", utils.CauseUeNotServedByAmf)
				return nil, problemDetails
			}
		}
		arrayOfAmfUpdateEventSubscriptionItem := (*modifySubscriptionRequest.ArrayOfAmfUpdateEventSubscriptionItem)[0]
		op := arrayOfAmfUpdateEventSubscriptionItem.GetOp()
		const pathPrefix = "/eventList/"
		path := arrayOfAmfUpdateEventSubscriptionItem.GetPath()
		if !strings.HasPrefix(path, pathPrefix) || len(path) <= len(pathPrefix) {
			problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Invalid subscription patch path")
			return nil, problemDetails
		}
		lists := subscription.GetEventList()
		eventlistLen := len(lists)

		indexStr := path[len(pathPrefix):]
		var index int
		if indexStr == "-" {
			// RFC 6902: "-" refers to the (nonexistent) member after the last array element and is only valid for "add"
			if op != "add" {
				problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Invalid subscription patch path index")
				return nil, problemDetails
			}
			index = eventlistLen
		} else {
			var err error
			index, err = strconv.Atoi(indexStr)
			if err != nil || index < 0 {
				problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Invalid subscription patch path index")
				return nil, problemDetails
			}
		}
		switch op {
		case "replace":
			if index >= eventlistLen {
				problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Invalid subscription patch path index")
				return nil, problemDetails
			}
			lists[index] = arrayOfAmfUpdateEventSubscriptionItem.GetValue()
			subscription.SetEventList(lists)
		case "remove":
			if index >= eventlistLen {
				problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Invalid subscription patch path index")
				return nil, problemDetails
			}
			subscription.SetEventList(append(lists[:index], lists[index+1:]...))
		case "add":
			// index == eventlistLen appends to the end of the array; RFC 6902 also allows the "-" path segment for this
			if index > eventlistLen {
				problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Invalid subscription patch path index")
				return nil, problemDetails
			}
			updatedList := make([]models.AmfEvent, 0, eventlistLen+1)
			updatedList = append(updatedList, lists[:index]...)
			updatedList = append(updatedList, arrayOfAmfUpdateEventSubscriptionItem.GetValue())
			updatedList = append(updatedList, lists[index:]...)
			subscription.SetEventList(updatedList)
		default:
			problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Unsupported subscription patch operation")
			return nil, problemDetails
		}
	} else {
		// Neither patch list is present: there is nothing to apply, so reject the request rather than
		// return a success that changed nothing. A ModifySubscriptionRequest must carry an option or
		// an event-subscription patch list.
		problemDetails := utils.ProblemDetailsMandatoryIeIncorrect("Modify request carries no option or event-subscription patch")
		return nil, problemDetails
	}

	// Serialization of the response runs in the HTTP layer after this function returns and the lock
	// is released, so hand it a copy whose mutable fields are independent of the shared object.
	updatedEventSubscription := models.NewAmfUpdatedEventSubscription(cloneEventSubscriptionForResponse(contextSubscription.EventSubscription))
	return updatedEventSubscription, nil
}

// cloneEventSubscriptionForResponse copies a subscription so the modify response owns the fields the
// procedure rewrites. The event list and options are reachable through pointers the shared object
// keeps mutating ("replace" patches the list in place and the option path rewrites the expiry), so
// cloning them lets the response be serialized after the subscription lock is released without
// racing a later modify.
func cloneEventSubscriptionForResponse(src models.AmfEventSubscription) models.AmfEventSubscription {
	cloned := src
	cloned.EventList = slices.Clone(src.GetEventList())
	if src.HasOptions() {
		options := src.GetOptions()
		cloned.SetOptions(options)
	}
	return cloned
}

func subReports(ue *context.AmfUe, subscriptionId string) {
	// Through the context rather than through the subscription this function used to fetch:
	// the counter is shared with the encoder that persists the UE, and a decrement applied
	// outside ue.Mutex raced it. It is also a read-modify-write, so two reports raised at
	// once could lose one.
	ue.DecrementRemainReports(subscriptionId)
}

// DO NOT handle AMFEVENTTYPE_PRESENCE_IN_AOI_REPORT and AMFEVENTTYPE_UES_IN_AREA_REPORT(about area)
func NewAmfEventReport(ue *context.AmfUe, Type models.AmfEventType, subscriptionId string) (
	report models.AmfEventReport, ok bool,
) {
	ueSubscription, ok := ue.GetEventSubscription(subscriptionId)
	if !ok {
		return report, ok
	}

	report.SetAnyUe(ueSubscription.AnyUe)
	report.SetSupi(ue.GetSupi())
	report.SetType(Type)
	report.SetTimeStamp(ueSubscription.Timestamp)
	report.SetState(models.AmfEventState{})
	mode := ueSubscription.EventSubscription.Options
	if mode == nil {
		report.State.SetActive(true)
	} else if mode.GetTrigger() == models.AMFEVENTTRIGGER_ONE_TIME {
		report.State.SetActive(false)
	} else if ueSubscription.RemainReports != nil && *ueSubscription.RemainReports <= 0 {
		report.State.SetActive(false)
	} else {
		expiry, remainDuration := getDuration(mode.Expiry)
		report.State.SetActive(expiry)
		if remainDuration != nil {
			report.State.SetRemainDuration(*remainDuration)
		}
		if expiry && ueSubscription.RemainReports != nil {
			report.State.SetRemainReports(*ueSubscription.RemainReports)
		}
	}

	switch Type {
	case models.AMFEVENTTYPE_LOCATION_REPORT:
		report.SetLocation(ue.GetLocation())
	// case models.AMFEVENTTYPE_PRESENCE_IN_AOI_REPORT:
	// report.AreaList = (*subscription.EventList)[eventIndex].AreaList
	case models.AMFEVENTTYPE_TIMEZONE_REPORT:
		report.SetTimezone(ue.TimeZone)
	case models.AMFEVENTTYPE_ACCESS_TYPE_REPORT:
		for accessType, state := range ue.State {
			if state.Is(context.Registered) {
				report.AccessTypeList = append(report.AccessTypeList, accessType)
			}
		}
	case models.AMFEVENTTYPE_REGISTRATION_STATE_REPORT:
		var rmInfos []models.RmInfo
		for accessType, state := range ue.State {
			rmInfo := models.RmInfo{
				RmState:    models.RMSTATE_DEREGISTERED,
				AccessType: accessType,
			}
			if state.Is(context.Registered) {
				rmInfo.RmState = models.RMSTATE_REGISTERED
			}
			rmInfos = append(rmInfos, rmInfo)
		}
		report.SetRmInfoList(rmInfos)
	case models.AMFEVENTTYPE_CONNECTIVITY_STATE_REPORT:
		report.SetCmInfoList(ue.GetCmInfo())
	case models.AMFEVENTTYPE_REACHABILITY_REPORT:
		report.SetReachability(ue.GetReachability())
	case models.AMFEVENTTYPE_COMMUNICATION_FAILURE_REPORT:
		// TODO : report.CommFailure
	case models.AMFEVENTTYPE_SUBSCRIPTION_ID_CHANGE:
		report.SetSubscriptionId(subscriptionId)
	case models.AMFEVENTTYPE_SUBSCRIPTION_ID_ADDITION:
		report.SetSubscriptionId(subscriptionId)
	}
	return report, ok
}

func getDuration(expiry *time.Time) (active bool, remainDuration *int32) {
	if expiry == nil {
		return true, nil
	}
	if time.Now().After(*expiry) {
		return false, nil
	}
	seconds := int32(time.Until(*expiry).Seconds())
	return true, &seconds
}
