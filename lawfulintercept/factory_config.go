// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import "github.com/omec-project/amf/factory"

// ConfigFromFactory maps the factory's li block onto this subsystem's configuration. It lives
// here rather than in the service package so the mapping can be tested where it is used.
func ConfigFromFactory(li *factory.Li, blockErr error) Config {
	dests := make([]Destination, 0, len(li.Destinations))
	for _, d := range li.Destinations {
		dests = append(dests, Destination{
			DID: d.DID, DeliveryType: d.DeliveryType, Address: d.Address,
		})
	}

	return Config{
		BlockError: blockErr,
		X1Listen:   li.X1Listen, MDF2: li.MDF2, NEID: li.NEID,
		Cert: li.Cert, Key: li.Key, CACert: li.CACert,
		Destinations: dests,
		AdmfURL:      li.AdmfURL, AdmfID: li.AdmfID, KeepaliveTimeout: li.KeepaliveTimeout,
		KeepaliveTimeP1:        li.KeepaliveTimeP1,
		KeepaliveTimeP3:        li.KeepaliveTimeP3,
		KeepaliveRetainTasking: li.KeepaliveRetainTasking,
		KeepaliveDisabled:      li.KeepaliveDisabled,
		X2X3KeepaliveEnabled:   li.X2X3KeepaliveEnabled,
		X2X3KeepaliveTimeP1:    li.X2X3KeepaliveTimeP1,
		X2X3KeepaliveTimeP2:    li.X2X3KeepaliveTimeP2,
		DeactivateAllTasks:     li.DeactivateAllTasks,
		RemoveAllDestinations:  li.RemoveAllDestinations,
	}
}
