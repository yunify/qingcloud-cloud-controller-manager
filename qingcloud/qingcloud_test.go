/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package qingcloud

import (
	"strings"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestReadConfig(t *testing.T) {
	_, err := readConfig(nil)
	if err == nil {
		t.Errorf("Should fail when no config is provided: %s", err)
	}

	cfg, err := readConfig(strings.NewReader(`
[Global]
qyConfigPath = /etc/qingcloud/client.yaml
zone = pek3a
 `))
	if err != nil {
		t.Fatalf("Should succeed when a valid config is provided: %s", err)
	}
	if cfg.Global.QYConfigPath != "/etc/qingcloud/client.yaml" {
		t.Errorf("incorrect config path: %s", cfg.Global.QYConfigPath)
	}
	if cfg.Global.Zone != "pek3a" {
		t.Errorf("incorrect zone: %s", cfg.Global.Zone)
	}
}

func TestZones(t *testing.T) {
	qc := QingCloud{zone: "ap1"}

	z, ok := qc.Zones()
	if !ok {
		t.Fatalf("Zones() returned false")
	}

	zone, err := z.GetZone()
	if err != nil {
		t.Fatalf("GetZone() returned error: %s", err)
	}

	if zone.Region != qc.zone {
		t.Fatalf("GetZone() returned wrong region (%s)", zone)
	}
}

//func TestLoadBalancer(t *testing.T) {
//	qc, err := getTestQingCloud()
//	if err != nil {
//		t.Fatal(err)
//	}
//	lbService, enable := qc.LoadBalancer()
//	assert.True(t, enable)
//
//	clusterName := "test_cluster"
//	service := &api.Service{
//		ObjectMeta: api.ObjectMeta{Name: "myservice", UID: "myserviceid",
//			Annotations: map[string]string{
//				ServiceAnnotationLoadBalancerEipIds:"eip-qrivjcov",
//			},
//		},
//		Spec: api.ServiceSpec{
//			Ports:[]api.ServicePort{
//				{
//					Protocol:api.ProtocolTCP,
//					Port:80,
//					NodePort:8080,
//				},
//			},
//		},
//	}
//	nodeNames := []string{}
//	lbStatus, err := lbService.EnsureLoadBalancer(clusterName, service, nodeNames)
//	assert.NoError(t, err)
//	assert.True(t, len(lbStatus.Ingress)  == 1)
//	lbStatus, exists, err := lbService.GetLoadBalancer(clusterName, service)
//	assert.NoError(t, err)
//	assert.True(t, exists)
//
//	nodeNames = append(nodeNames, "i-3810y27u")
//	err = lbService.UpdateLoadBalancer(clusterName, service, nodeNames)
//	assert.NoError(t, err)
//
//	nodeNames = append(nodeNames, "i-cehv89m6")
//	err = lbService.UpdateLoadBalancer(clusterName, service, nodeNames)
//	assert.NoError(t, err)
//
//	time.Sleep(2*time.Second)
//
//	err = lbService.EnsureLoadBalancerDeleted(clusterName, service)
//	for err != nil {
//		err = lbService.EnsureLoadBalancerDeleted(clusterName, service)
//		time.Sleep(2*time.Second)
//	}
//	assert.NoError(t, err)
//
//	lbStatus, exists, err = lbService.GetLoadBalancer(clusterName, service)
//	assert.NoError(t, err)
//	assert.False(t, exists)
//}
//
//func TestVolume(t *testing.T) {
//
//	provider, err := getTestQingCloud()
//	if err != nil {
//		t.Fatal(err)
//	}
//	qc := provider.(*QingCloud)
//	volumeID, err := qc.CreateVolume(&VolumeOptions{CapacityGB:10, VolumeType:0})
//	assert.NoError(t, err)
//	instanceID := "i-3810y27u"
//	dev, err := qc.AttachVolume(volumeID, instanceID)
//	assert.NoError(t, err)
//	println("volume", volumeID, dev)
//
//	attached, err := qc.VolumeIsAttached(volumeID, instanceID)
//	assert.NoError(t, err)
//	assert.True(t, attached)
//
//	attachedMap, err := qc.DisksAreAttached([]string{volumeID},instanceID)
//	assert.NoError(t, err)
//	assert.True(t, attachedMap[volumeID])
//
//	err = qc.DetachVolume(volumeID, instanceID)
//	assert.NoError(t, err)
//
//	attached, err = qc.VolumeIsAttached(volumeID, instanceID)
//	assert.NoError(t, err)
//	assert.False(t, attached)
//
//	attachedMap, err = qc.DisksAreAttached([]string{volumeID},instanceID)
//	assert.NoError(t, err)
//	assert.False(t, attachedMap[volumeID])
//
//	found, err := qc.DeleteVolume(volumeID)
//	for err != nil {
//		found, err = qc.DeleteVolume(volumeID)
//		time.Sleep(2*time.Second)
//	}
//	assert.NoError(t, err)
//	assert.True(t, found)
//
//}

func TestStringArrayPtr(t *testing.T) {
	ss := []string{"a","b"}
	ssa := stringArrayPtr(ss)
	for i,s := range ss {
		assert.Equal(t, *ssa[i], s)
	}
}