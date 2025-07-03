// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package metricinfo

type CoreSubscriber struct {
	Version     int    `json:"version,omitempty"`
	Imsi        string `json:"imsi,omitempty"` // key
	SmfId       string `json:"smfId,omitempty"`
	SmfIp       string `json:"smfIp,omitempty"`
	SmfSubState string `json:"smfSubState,omitempty"` // Connected, Idle, DisConnected
	IPAddress   string `json:"ipaddress,omitempty"`
	Dnn         string `json:"dnn,omitempty"`
	Slice       string `json:"slice,omitempty"`
	LSEID       int    `json:"lseid,omitempty"`
	RSEID       int    `json:"rseid,omitempty"`
	UpfName     string `json:"upfid,omitempty"`
	UpfAddr     string `json:"upfAddr,omitempty"`
	AmfId       string `json:"amfId,omitempty"`
	Guti        string `json:"guti,omitempty"`
	Tmsi        int32  `json:"tmsi,omitempty"`
	AmfNgapId   int64  `json:"amfngapId,omitempty"`
	RanNgapId   int64  `json:"ranngapId,omitempty"`
	AmfSubState string `json:"amfSubState,omitempty"` // RegisteredC, RegisteredI, DeRegistered, Deleted
	GnbId       string `json:"gnbid,omitempty"`
	TacId       string `json:"tacid,omitempty"`
	AmfIp       string `json:"amfIp,omitempty"`
	UeState     string `json:"ueState,omitempty"`
}

type CoreMsgType struct {
	MsgType    string `json:"msgType,omitempty"`
	SourceNfId string `json:"sourceNfId,omitempty"`
}

type CoreEventType int64

const (
	CSubscriberEvt CoreEventType = iota
	CMsgTypeEvt
	CNfStatusEvt
)

func (e CoreEventType) String() string {
	switch e {
	case CSubscriberEvt:
		return "SubscriberEvt"
	case CMsgTypeEvt:
		return "MsgTypeEvt"
	case CNfStatusEvt:
		return "CNfStatusEvt"
	}
	return "Unknown"
}

type NfStatusType string

const (
	NfStatusConnected    NfStatusType = "Connected"
	NfStatusDisconnected NfStatusType = "Disconnected"
)

type NfType string

const (
	NfTypeSmf NfType = "SMF"
	NfTypeAmf NfType = "AMF"
	NfTypeUPF NfType = "UPF"
	NfTypeGnb NfType = "GNB"
	NfTypeEnd NfType = "Invalid"
)

type CNfStatus struct {
	NfType   NfType       `json:"nfType,omitempty"`
	NfStatus NfStatusType `json:"nfStatus,omitempty"`
	NfName   string       `json:"nfName,omitempty"`
}

type SubscriberOp uint

const (
	SubsOpAdd SubscriberOp = iota + 1
	SubsOpMod
	SubsOpDel
)

type CoreSubscriberData struct {
	Subscriber CoreSubscriber `json:"subscriber,omitempty"`
	Operation  SubscriberOp   `json:"subsOp,omitempty"`
}

// Sent by NFs(Producers) and received by Metric Function
type MetricEvent struct {
	EventType      CoreEventType      `json:"eventType,omitempty"`
	SubscriberData CoreSubscriberData `json:"subscriberData,omitempty"`
	MsgType        CoreMsgType        `json:"coreMsgType,omitempty"`
	NfStatusData   CNfStatus          `json:"nfStatusData"`
}

type SmfMsgType uint64

const (
	Smf_msg_type_invalid SmfMsgType = iota
	Smf_msg_type_pdu_sess_create_req
	Smf_msg_type_pdu_sess_create_rsp_success
	Smf_msg_type_pdu_sess_create_rsp_failure
	Smf_msg_type_pdu_sess_modify_req
	Smf_msg_type_pdu_sess_modify_rsp_success
	Smf_msg_type_pdu_sess_modify_rsp_failure
	Smf_msg_type_pdu_sess_release_req
	Smf_msg_type_pdu_sess_release_rsp_success
	Smf_msg_type_pdu_sess_release_rsp_failure
	Smf_msg_type_n1n2_transfer_req
	Smf_msg_type_n1n2_transfer_rsp_success
	Smf_msg_type_n1n2_transfer_rsp_failure
	Smf_msg_type_smpolicy_create_req
	Smf_msg_type_smpolicy_create_rsp_success
	Smf_msg_type_smpolicy_create_rsp_failure
	Smf_msg_type_pfcp_sess_estab_req
	Smf_msg_type_pfcp_sess_estab_rsp_success
	Smf_msg_type_pfcp_sess_estab_rsp_failure
	Smf_msg_type_pfcp_sess_modify_req
	Smf_msg_type_pfcp_sess_modify_rsp_success
	Smf_msg_type_pfcp_sess_modify_rsp_failure
	Smf_msg_type_pfcp_sess_release_req
	Smf_msg_type_pfcp_sess_release_rsp_success
	Smf_msg_type_pfcp_sess_release_rsp_failure
	Smf_msg_type_pfcp_association_req
	Smf_msg_type_pfcp_association_rsp_success
	Smf_msg_type_pfcp_association_rsp_failure
	Smf_msg_type_pfcp_heartbeat_req
	Smf_msg_type_pfcp_heartbeat_rsp_success
	Smf_msg_type_pfcp_heartbeat_rsp_failure
	Smf_msg_type_udm_get_smdata_req
	Smf_msg_type_udm_get_smdata_rsp_success
	Smf_msg_type_udm_get_smdata_rsp_failure
	Smf_msg_type_nrf_discovery_amf_req
	Smf_msg_type_nrf_discovery_amf_rsp_success
	Smf_msg_type_nrf_discovery_amf_rsp_failure
	Smf_msg_type_nrf_discovery_pcf_req
	Smf_msg_type_nrf_discovery_pcf_rsp_success
	Smf_msg_type_nrf_discovery_pcf_rsp_failure
	Smf_msg_type_nrf_discovery_udm_req
	Smf_msg_type_nrf_discovery_udm_rsp_success
	Smf_msg_type_nrf_discovery_udm_rsp_failure
	Smf_msg_type_nrf_register_smf_req
	Smf_msg_type_nrf_register_smf_rsp_success
	Smf_msg_type_nrf_register_smf_rsp_failure
	Smf_msg_type_nrf_deregister_smf_req
	Smf_msg_type_nrf_deregister_smf_rsp_success
	Smf_msg_type_nrf_deregister_smf_rsp_failure
)

func (t SmfMsgType) String() string {
	switch t {
	case Smf_msg_type_pdu_sess_create_req:
		return "smf_pdu_sess_create_req"
	case Smf_msg_type_pdu_sess_create_rsp_success:
		return "smf_pdu_sess_create_rsp_success"
	case Smf_msg_type_pdu_sess_create_rsp_failure:
		return "smf_pdu_sess_create_rsp_failure"
	case Smf_msg_type_pdu_sess_modify_req:
		return "smf_pdu_sess_modify_req"
	case Smf_msg_type_pdu_sess_modify_rsp_success:
		return "smf_pdu_sess_modify_rsp_success"
	case Smf_msg_type_pdu_sess_modify_rsp_failure:
		return "smf_pdu_sess_modify_rsp_failure"
	case Smf_msg_type_pdu_sess_release_req:
		return "smf_pdu_sess_release_req"
	case Smf_msg_type_pdu_sess_release_rsp_success:
		return "smf_pdu_sess_release_rsp_success"
	case Smf_msg_type_pdu_sess_release_rsp_failure:
		return "smf_pdu_sess_release_rsp_failure"
	case Smf_msg_type_n1n2_transfer_req:
		return "smf_n1n2_transfer_req"
	case Smf_msg_type_n1n2_transfer_rsp_success:
		return "smf_n1n2_transfer_rsp_success"
	case Smf_msg_type_n1n2_transfer_rsp_failure:
		return "smf_n1n2_transfer_rsp_failure"
	case Smf_msg_type_smpolicy_create_req:
		return "smf_smpolicy_create_req"
	case Smf_msg_type_smpolicy_create_rsp_success:
		return "smf_smpolicy_create_rsp_success"
	case Smf_msg_type_smpolicy_create_rsp_failure:
		return "smf_smpolicy_create_rsp_failure"
	case Smf_msg_type_pfcp_sess_estab_req:
		return "smf_pfcp_sess_estab_req"
	case Smf_msg_type_pfcp_sess_estab_rsp_success:
		return "smf_pfcp_sess_estab_rsp_success"
	case Smf_msg_type_pfcp_sess_estab_rsp_failure:
		return "smf_pfcp_sess_estab_rsp_failure"
	case Smf_msg_type_pfcp_sess_modify_req:
		return "smf_pfcp_sess_modify_req"
	case Smf_msg_type_pfcp_sess_modify_rsp_success:
		return "smf_pfcp_sess_modify_rsp_success"
	case Smf_msg_type_pfcp_sess_modify_rsp_failure:
		return "smf_pfcp_sess_modify_rsp_failure"
	case Smf_msg_type_pfcp_sess_release_req:
		return "smf_pfcp_sess_release_req"
	case Smf_msg_type_pfcp_sess_release_rsp_success:
		return "smf_pfcp_sess_release_rsp_success"
	case Smf_msg_type_pfcp_sess_release_rsp_failure:
		return "smf_pfcp_sess_release_rsp_failure"
	case Smf_msg_type_pfcp_association_req:
		return "smf_pfcp_association_req"
	case Smf_msg_type_pfcp_association_rsp_success:
		return "smf_pfcp_association_rsp_success"
	case Smf_msg_type_pfcp_association_rsp_failure:
		return "smf_pfcp_association_rsp_failure"
	case Smf_msg_type_pfcp_heartbeat_req:
		return "smf_pfcp_heartbeat_req"
	case Smf_msg_type_pfcp_heartbeat_rsp_success:
		return "smf_pfcp_heartbeat_rsp_success"
	case Smf_msg_type_pfcp_heartbeat_rsp_failure:
		return "smf_pfcp_heartbeat_rsp_failure"
	case Smf_msg_type_udm_get_smdata_req:
		return "smf_udm_get_smdata_req"
	case Smf_msg_type_udm_get_smdata_rsp_success:
		return "smf_udm_get_smdata_rsp_success"
	case Smf_msg_type_udm_get_smdata_rsp_failure:
		return "smf_udm_get_smdata_rsp_failure"
	case Smf_msg_type_nrf_discovery_amf_req:
		return "smf_nrf_discovery_amf_req"
	case Smf_msg_type_nrf_discovery_amf_rsp_success:
		return "smf_nrf_discovery_amf_rsp_success"
	case Smf_msg_type_nrf_discovery_amf_rsp_failure:
		return "smf_nrf_discovery_amf_rsp_failure"
	case Smf_msg_type_nrf_discovery_pcf_req:
		return "smf_nrf_discovery_pcf_req"
	case Smf_msg_type_nrf_discovery_pcf_rsp_success:
		return "smf_nrf_discovery_pcf_rsp_success"
	case Smf_msg_type_nrf_discovery_pcf_rsp_failure:
		return "smf_nrf_discovery_pcf_rsp_failure"
	case Smf_msg_type_nrf_discovery_udm_req:
		return "smf_nrf_discovery_udm_req"
	case Smf_msg_type_nrf_discovery_udm_rsp_success:
		return "smf_nrf_discovery_udm_rsp_success"
	case Smf_msg_type_nrf_discovery_udm_rsp_failure:
		return "smf_nrf_discovery_udm_rsp_failure"
	case Smf_msg_type_nrf_register_smf_req:
		return "smf_nrf_register_smf_req"
	case Smf_msg_type_nrf_register_smf_rsp_success:
		return "smf_nrf_register_smf_rsp_success"
	case Smf_msg_type_nrf_register_smf_rsp_failure:
		return "smf_nrf_register_smf_rsp_failure"
	case Smf_msg_type_nrf_deregister_smf_req:
		return "smf_nrf_deregister_smf_req"
	case Smf_msg_type_nrf_deregister_smf_rsp_success:
		return "smf_nrf_deregister_smf_rsp_success"
	case Smf_msg_type_nrf_deregister_smf_rsp_failure:
		return "smf_nrf_deregister_smf_rsp_failure"
	default:
		return "unknown smf msg type"
	}
}

type AmfMsgType uint64

const (
	Amf_msg_type_invalid AmfMsgType = iota
	Amf_msg_type_ngap_ng_setup_req
	Amf_msg_type_ngap_ng_setup_rsp
	Amf_msg_type_ngap_ng_setup_failure
	Amf_msg_type_ngap_init_ue
	Amf_msg_type_ngap_ul_nas_transport
	Amf_msg_type_ngap_reset_req
	Amf_msg_type_ngap_reset_ack
	Amf_msg_type_ngap_handover_cancel
	Amf_msg_type_ngap_ue_ctxt_rel_req
	Amf_msg_type_ngap_ue_ctxt_rel_complete
	Amf_msg_type_ngap_nas_non_dlvry_ind
	Amf_msg_type_ngap_location_report_fail_ind
	Amf_msg_type_ngap_error_ind
	Amf_msg_type_ngap_ue_radio_cap_ind
	Amf_msg_type_ngap_handover_notify
	Amf_msg_type_ngap_handover_prep
	Amf_msg_type_ngap_ran_config_update
	Amf_msg_type_ngap_rrc_inactive_trans_report
	Amf_msg_type_ngap_pdu_sess_resource_notify
	Amf_msg_type_ngap_path_switch_req
	Amf_msg_type_ngap_pdu_sess_resource_mod_ind
	Amf_msg_type_ngap_resource_rel
	Amf_msg_type_ngap_ue_radio_cap_check
	Amf_msg_type_ngap_amd_config_update
	Amf_msg_type_ngap_initial_ctxt_rsp
	Amf_msg_type_ngap_ue_ctxt_mod_rsp
	Amf_msg_type_ngap_resource_setup_rsp
	Amf_msg_type_ngap_pdu_sess_resource_mod_rsp
	Amf_msg_type_ngap_handover_req_ack
	Amf_msg_type_ngap_amf_config_update_failure
	Amf_msg_type_ngap_initial_ctxt_setup_failure
	Amf_msg_type_ngap_handover_failure
	Amf_msg_type_ngap_ue_ctxt_mod_failure
)

func (t AmfMsgType) String() string {
	switch t {
	case Amf_msg_type_ngap_ng_setup_req:
		return "amf_ngap_ng_setup_req"
	case Amf_msg_type_ngap_ng_setup_rsp:
		return "amf_ngap_ng_setup_rsp"
	case Amf_msg_type_ngap_ng_setup_failure:
		return "amf_ngap_ng_setup_failure"
	case Amf_msg_type_ngap_init_ue:
		return "amf_ngap_init_ue"
	case Amf_msg_type_ngap_ul_nas_transport:
		return "amf_ngap_ul_nas_transport"
	case Amf_msg_type_ngap_reset_req:
		return "amf_ngap_reset_req"
	case Amf_msg_type_ngap_reset_ack:
		return "amf_ngap_reset_ack"
	case Amf_msg_type_ngap_handover_cancel:
		return "amf_ngap_handover_cancel"
	case Amf_msg_type_ngap_ue_ctxt_rel_req:
		return "amf_ngap_ue_ctxt_rel_req"
	case Amf_msg_type_ngap_ue_ctxt_rel_complete:
		return "amf_ngap_ue_ctxt_rel_complete"
	case Amf_msg_type_ngap_nas_non_dlvry_ind:
		return "amf_ngap_nas_non_dlvry_ind"
	case Amf_msg_type_ngap_location_report_fail_ind:
		return "amf_ngap_location_report_fail_ind"
	case Amf_msg_type_ngap_error_ind:
		return "amf_ngap_error_ind"
	case Amf_msg_type_ngap_ue_radio_cap_ind:
		return "amf_ngap_ue_radio_cap_ind"
	case Amf_msg_type_ngap_handover_notify:
		return "amf_ngap_handover_notify"
	case Amf_msg_type_ngap_handover_prep:
		return "amf_ngap_handover_prep"
	case Amf_msg_type_ngap_ran_config_update:
		return "amf_ngap_ran_config_update"
	case Amf_msg_type_ngap_rrc_inactive_trans_report:
		return "amf_ngap_rrc_inactive_trans_report"
	case Amf_msg_type_ngap_pdu_sess_resource_notify:
		return "amf_ngap_pdu_sess_resource_notify"
	case Amf_msg_type_ngap_path_switch_req:
		return "amf_ngap_path_switch_req"
	case Amf_msg_type_ngap_pdu_sess_resource_mod_ind:
		return "amf_ngap_pdu_sess_resource_mod_ind"
	case Amf_msg_type_ngap_resource_rel:
		return "amf_ngap_resource_rel"
	case Amf_msg_type_ngap_ue_radio_cap_check:
		return "amf_ngap_ue_radio_cap_check"
	case Amf_msg_type_ngap_amd_config_update:
		return "amf_ngap_amd_config_update"
	case Amf_msg_type_ngap_initial_ctxt_rsp:
		return "amf_ngap_initial_ctxt_rsp"
	case Amf_msg_type_ngap_ue_ctxt_mod_rsp:
		return "amf_ngap_ue_ctxt_mod_rsp"
	case Amf_msg_type_ngap_resource_setup_rsp:
		return "amf_ngap_resource_setup_rsp"
	case Amf_msg_type_ngap_pdu_sess_resource_mod_rsp:
		return "amf_ngap_pdu_sess_resource_mod_rsp"
	case Amf_msg_type_ngap_handover_req_ack:
		return "amf_ngap_handover_req_ack"
	case Amf_msg_type_ngap_amf_config_update_failure:
		return "amf_ngap_amf_config_update_failure"
	case Amf_msg_type_ngap_initial_ctxt_setup_failure:
		return "amf_ngap_initial_ctxt_setup_failure"
	case Amf_msg_type_ngap_handover_failure:
		return "amf_ngap_handover_failure"
	case Amf_msg_type_ngap_ue_ctxt_mod_failure:
		return "amf_ngap_ue_ctxt_mod_failure"
	default:
		return "unknown amf msg type"
	}
}
