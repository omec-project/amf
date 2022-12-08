// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
//

package util

import (
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/util/drsm"
)

type MockDrsmInterface interface {
	AllocateInt32ID() (int32, error)
	ReleaseInt32ID(id int32) error
	FindOwnerInt32ID(id int32) (*drsm.PodId, error)
	AcquireIp(pool string) (string, error)
	ReleaseIp(pool, ip string) error
	CreateIpPool(poolName string, ipPool string) error
	DeleteIpPool(poolName string) error
}
type MockDrsm struct {
}

func MockDrsmInit() (drsm.DrsmInterface, error) {
	//db := drsm.DbInfo{"mongodb://mongodb", "amf"}
	//podId := drsm.PodId{"amf-instance1", "1.1.1.1"}
	//opt := &drsm.Options{ResIdSize: 24, Mode: drsm.ResourceClient}
	d := &MockDrsm{}
	return d, nil
}

func (d *MockDrsm) AllocateInt32ID() (int32, error) {
	logger.AppLog.Info("MockAllocate")
	return 1, nil
}

func (d *MockDrsm) ReleaseInt32ID(id int32) error {
	logger.AppLog.Info("MockRelease")
	return nil
}

func (d *MockDrsm) FindOwnerInt32ID(id int32) (*drsm.PodId, error) {
	return nil, nil
}

func (d *MockDrsm) AcquireIp(pool string) (string, error) {
	return "", nil
}

func (d *MockDrsm) ReleaseIp(pool, ip string) error {
	return nil
}

func (d *MockDrsm) CreateIpPool(poolName string, ipPool string) error {
	return nil
}

func (d *MockDrsm) DeleteIpPool(poolName string) error {
	return nil
}
