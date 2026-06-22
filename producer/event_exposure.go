// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package producer

import (
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2"
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
		return httpwrapper.NewResponse(http.StatusInternalServerError, nil, problemDetails)
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
	// TODO: GA: Review the constructor of NewExtAmfEventSubscription. Is there anything else missing?
	extAmfEventSubscription := models.NewExtAmfEventSubscription(contextEventSubscription.EventSubscription.GetEventList(), contextEventSubscription.EventSubscription.GetEventNotifyUri(), contextEventSubscription.EventSubscription.GetNotifyCorrelationId(), contextEventSubscription.EventSubscription.GetNfId())
	ueEventSubscription.EventSubscription = extAmfEventSubscription
	ueEventSubscription.Timestamp = time.Now().UTC()

	if subscription.Options != nil && subscription.Options.Trigger == models.AMFEVENTTRIGGER_CONTINUOUS {
		ueEventSubscription.RemainReports = subscription.Options.MaxReports
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
		amfSelf.UePool.Range(func(key, value interface{}) bool {
			ue := value.(*context.AmfUe)
			ue.EventSubscriptionsInfo[newSubscriptionID] = new(context.AmfUeEventSubscription)
			*ue.EventSubscriptionsInfo[newSubscriptionID] = ueEventSubscription
			contextEventSubscription.UeSupiList = append(contextEventSubscription.UeSupiList, ue.Supi)
			return true
		})
	} else if subscription.GetGroupId() != "" {
		contextEventSubscription.IsGroupUe = true
		ueEventSubscription.AnyUe = true
		amfSelf.UePool.Range(func(key, value interface{}) bool {
			ue := value.(*context.AmfUe)
			if ue.GroupID == subscription.GetGroupId() {
				ue.EventSubscriptionsInfo[newSubscriptionID] = new(context.AmfUeEventSubscription)
				*ue.EventSubscriptionsInfo[newSubscriptionID] = ueEventSubscription
				contextEventSubscription.UeSupiList = append(contextEventSubscription.UeSupiList, ue.Supi)
			}
			return true
		})
	} else {
		if ue, ok := amfSelf.AmfUeFindBySupi(subscription.GetSupi()); !ok {
			problemDetails := utils.ProblemDetailsWithCause("UE not served by AMF", http.StatusForbidden, "UE is not served by this AMF", utils.CauseUeNotServedByAmf)
			return nil, problemDetails
		} else {
			ue.EventSubscriptionsInfo[newSubscriptionID] = new(context.AmfUeEventSubscription)
			*ue.EventSubscriptionsInfo[newSubscriptionID] = ueEventSubscription
			contextEventSubscription.UeSupiList = append(contextEventSubscription.UeSupiList, ue.Supi)
		}
	}

	// delete subscription
	if subscription.Options != nil {
		contextEventSubscription.Expiry = subscription.Options.Expiry
	}
	amfSelf.NewEventSubscription(newSubscriptionID, &contextEventSubscription)

	// build response
	createdEventSubscription := models.NewAmfCreatedEventSubscription(subscription, newSubscriptionID)

	// for immediate use
	if subscription.GetAnyUE() {
		amfSelf.UePool.Range(func(key, value interface{}) bool {
			ue := value.(*context.AmfUe)
			if isImmediate {
				subReports(ue, newSubscriptionID)
			}
			for i, flag := range immediateFlags {
				if flag {
					report, ok := NewAmfEventReport(ue, (subscription.EventList)[i].Type, newSubscriptionID)
					if ok {
						reportlist = append(reportlist, report)
					}
				}
			}
			// delete subscription
			if reportlistLen := len(reportlist); reportlistLen > 0 && (!reportlist[reportlistLen-1].State.Active) {
				delete(ue.EventSubscriptionsInfo, newSubscriptionID)
			}
			return true
		})
	} else if subscription.GetGroupId() != "" {
		amfSelf.UePool.Range(func(key, value interface{}) bool {
			ue := value.(*context.AmfUe)
			if isImmediate {
				subReports(ue, newSubscriptionID)
			}
			if ue.GroupID == subscription.GetGroupId() {
				for i, flag := range immediateFlags {
					if flag {
						report, ok := NewAmfEventReport(ue, (subscription.EventList)[i].Type, newSubscriptionID)
						if ok {
							reportlist = append(reportlist, report)
						}
					}
				}
				// delete subscription
				if reportlistLen := len(reportlist); reportlistLen > 0 && (!reportlist[reportlistLen-1].State.Active) {
					delete(ue.EventSubscriptionsInfo, newSubscriptionID)
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
				report, ok := NewAmfEventReport(ue, (subscription.EventList)[i].Type, newSubscriptionID)
				if ok {
					reportlist = append(reportlist, report)
				}
			}
		}
		// delete subscription
		if reportlistLen := len(reportlist); reportlistLen > 0 && (!reportlist[reportlistLen-1].State.Active) {
			delete(ue.EventSubscriptionsInfo, newSubscriptionID)
		}
	}
	if len(reportlist) > 0 {
		createdEventSubscription.ReportList = reportlist
		// delete subscription
		if !reportlist[0].State.Active {
			amfSelf.DeleteEventSubscription(newSubscriptionID)
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

	for _, supi := range subscription.UeSupiList {
		if ue, ok := amfSelf.AmfUeFindBySupi(supi); ok {
			delete(ue.EventSubscriptionsInfo, subscriptionID)
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
		return httpwrapper.NewResponse(http.StatusInternalServerError, nil, problemDetails)
	}
}

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

	if modifySubscriptionRequest.ArrayOfAmfUpdateEventOptionItem != nil {
		expiry0 := (*modifySubscriptionRequest.ArrayOfAmfUpdateEventOptionItem)[0].GetValue()
		contextSubscription.Expiry = &expiry0
	} else if modifySubscriptionRequest.ArrayOfAmfUpdateEventSubscriptionItem != nil {
		subscription := &contextSubscription.EventSubscription
		if !contextSubscription.IsAnyUe && !contextSubscription.IsGroupUe {
			if _, ok := amfSelf.AmfUeFindBySupi(subscription.GetSupi()); !ok {
				problemDetails := utils.ProblemDetailsWithCause("UE not served by AMF", http.StatusForbidden, "UE is not served by this AMF", "UE_NOT_SERVED_BY_AMF")
				return nil, problemDetails
			}
		}
		arrayOfAmfUpdateEventSubscriptionItem := (*modifySubscriptionRequest.ArrayOfAmfUpdateEventSubscriptionItem)[0]
		op := arrayOfAmfUpdateEventSubscriptionItem.GetOp()
		index, err := strconv.Atoi(arrayOfAmfUpdateEventSubscriptionItem.Path[11:])
		if err != nil {
			problemDetails := utils.ProblemDetailsWithCause("Unspecified NF failure", http.StatusInternalServerError, "Failed to parse subscription path", "UNSPECIFIED_NF_FAILURE")
			return nil, problemDetails
		}
		lists := (subscription.EventList)
		eventlistLen := len(subscription.EventList)
		switch op {
		case "replace":
			event := arrayOfAmfUpdateEventSubscriptionItem.GetValue()
			if index < eventlistLen {
				(subscription.EventList)[index] = event
			}
		case "remove":
			if index < eventlistLen {
				subscription.EventList = append(lists[:index], lists[index+1:]...)
			}
		case "add":
			subscription.EventList = append(lists, arrayOfAmfUpdateEventSubscriptionItem.GetValue())
		}
	}

	updatedEventSubscription := models.NewAmfUpdatedEventSubscription(contextSubscription.EventSubscription)
	return updatedEventSubscription, nil
}

func subReports(ue *context.AmfUe, subscriptionId string) {
	remainReport := ue.EventSubscriptionsInfo[subscriptionId].RemainReports
	if remainReport == nil {
		return
	}
	*remainReport--
}

// DO NOT handle AMFEVENTTYPE_PRESENCE_IN_AOI_REPORT and AMFEVENTTYPE_UES_IN_AREA_REPORT(about area)
func NewAmfEventReport(ue *context.AmfUe, Type models.AmfEventType, subscriptionId string) (
	report models.AmfEventReport, ok bool,
) {
	ueSubscription, ok := ue.EventSubscriptionsInfo[subscriptionId]
	if !ok {
		return report, ok
	}

	report.AnyUe = openapi.PtrBool(ueSubscription.AnyUe)
	report.Supi = openapi.PtrString(ue.Supi)
	report.Type = Type
	report.TimeStamp = ueSubscription.Timestamp
	report.State = models.AmfEventState{}
	mode := ueSubscription.EventSubscription.Options
	if mode == nil {
		report.State.SetActive(true)
	} else if mode.Trigger == models.AMFEVENTTRIGGER_ONE_TIME {
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
		report.Location = &ue.Location
	// case models.AMFEVENTTYPE_PRESENCE_IN_AOI_REPORT:
	// report.AreaList = (*subscription.EventList)[eventIndex].AreaList
	case models.AMFEVENTTYPE_TIMEZONE_REPORT:
		report.Timezone = openapi.PtrString(ue.TimeZone)
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
		report.RmInfoList = rmInfos
	case models.AMFEVENTTYPE_CONNECTIVITY_STATE_REPORT:
		report.CmInfoList = ue.GetCmInfo()
	case models.AMFEVENTTYPE_REACHABILITY_REPORT:
		report.Reachability = &ue.Reachability
	// TODO: GA: Need to check the content of SubscribedData
	// case models.AMFEVENTTYPE_SUBSCRIBED_DATA_REPORT:
	// 	report.SubscribedData = &ue.SubscribedData
	case models.AMFEVENTTYPE_COMMUNICATION_FAILURE_REPORT:
		// TODO : report.CommFailure
	case models.AMFEVENTTYPE_SUBSCRIPTION_ID_CHANGE:
		report.SubscriptionId = openapi.PtrString(subscriptionId)
	case models.AMFEVENTTYPE_SUBSCRIPTION_ID_ADDITION:
		report.SubscriptionId = openapi.PtrString(subscriptionId)
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
