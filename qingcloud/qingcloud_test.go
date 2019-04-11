// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package qingcloud

import (
	"context"
	"fmt"
	"strings"
	"testing"

	//"k8s.io/client-go/pkg/api"
	//"github.com/stretchr/testify/assert"
	"bytes"
	"encoding/json"

	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/api/core/v1"
	machineryv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getMockupQingCloud() (*qcservice.DescribeLoadBalancersOutput, *qcservice.DescribeLoadBalancerListenersOutput, error) {
	fakedLoadBalancerOutput := `
	{
		"action": "DescribeLoadBalancersResponse",
		"total_count": 1,
		"loadbalancer_set": [
		  {
			"is_applied": 1,
			"vxnet_id": "vxnet-0",
			"console_id": "qingcloud",
			"cluster": [
			  {
				"eip_name": "ip01",
				"eip_addr": "139.198.11.67",
				"eip_id": "eip-ekre2wcl",
				"instances": [
				  {
					"instance_id": "i-xngoyk71",
					"vgw_mgmt_ip": "100.64.0.10"
				  }
				]
			  }
			],
			"create_time": "2017-05-31T06:06:45Z",
			"rsyslog": "",
			"owner": "usr-gEDGfgZJ",
			"place_group_id": "plg-00000000",
			"features": 0,
			"sub_code": 0,
			"security_group_id": "sg-lufujop2",
			"loadbalancer_type": 0,
			"loadbalancer_name": "lb02",
			"memory": 320,
			"status_time": "2017-08-28T03:17:48Z",
			"node_count": 1,
			"status": "active",
			"description": "",
			"tags": [],
			"transition_status": "",
			"eips": [],
			"controller": "self",
			"repl": "rpp-00000000",
			"private_ips": [
			  "198.19.0.100"
			],
			"hypervisor": "",
			"loadbalancer_id": "lb-lmld6diw",
			"root_user_id": "usr-gEDGfgZJ",
			"http_header_size": null,
			"mode": 1,
			"cpu": 1
		  }
		],
		"ret_code": 0
	  }`
	fakedLoadBalancerListenersOutput := `{
		"action": "DescribeLoadBalancerListenersResponse",
		"total_count": 1,
		"loadbalancer_listener_set": [
		  {
			"forwardfor": 0,
			"listener_option": 2,
			"listener_protocol": "http",
			"server_certificate_id": "",
			"backend_protocol": "http",
			"healthy_check_method": "tcp",
			"session_sticky": "insert|",
			"controller": "self",
			"console_id": "qingcloud",
			"disabled": 0,
			"balance_mode": "roundrobin",
			"root_user_id": "usr-gEDGfgZJ",
			"create_time": "2017-05-31T06:09:39Z",
			"healthy_check_option": "20|5|2|5",
			"timeout": 50,
			"owner": "usr-gEDGfgZJ",
			"loadbalancer_listener_id": "lbl-0b307ovo",
			"loadbalancer_listener_name": "httpMonitor",
			"loadbalancer_id": "lb-lmld6diw",
			"listener_port": 80
		  }
		],
		"ret_code": 0
	  }`
	bufferLB := bytes.NewBufferString(fakedLoadBalancerOutput)
	respLB := qcservice.DescribeLoadBalancersOutput{}
	bufferListeners := bytes.NewBufferString(fakedLoadBalancerListenersOutput)
	respListeners := qcservice.DescribeLoadBalancerListenersOutput{}
	err := json.Unmarshal(bufferLB.Bytes(), &respLB)
	if err != nil {
		return nil, nil, err
	}
	err = json.Unmarshal(bufferListeners.Bytes(), &respListeners)
	if err != nil {
		return nil, nil, err
	}
	return &respLB, &respListeners, nil
}
func getMockupServiceSpec() (srEip *v1.Service, srVxNet *v1.Service) {
	serviceEip := &v1.Service{
		ObjectMeta: machineryv1.ObjectMeta{Name: "myservice", UID: "myserviceid",
			Annotations: map[string]string{
				ServiceAnnotationLoadBalancerEipIds: "eip-qrivjcov",
				ServiceAnnotationLoadBalancerType:   "0",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     80,
					NodePort: 8080,
				},
			},
		},
	}

	serviceVxnet := &v1.Service{
		ObjectMeta: machineryv1.ObjectMeta{Name: "myservice", UID: "myserviceid",
			Annotations: map[string]string{
				ServiceAnnotationLoadBalancerVxnetId: "vxnet-umltx6a",
				ServiceAnnotationLoadBalancerType:    "0",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     80,
					NodePort: 8080,
				},
			},
		},
	}
	return serviceEip, serviceVxnet
}
func TestReadConfig(t *testing.T) {
	_, err := readConfig(nil)
	if err == nil {
		t.Errorf("Should fail when no config is provided: %s", err)
	}

	cfg, err := readConfig(strings.NewReader(`
[Global]
qyConfigPath = /etc/qingcloud/client.yaml
zone = pek3a
defaultVxNetForLB = vxnet-umltx6a
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
	if cfg.Global.DefaultVxNetForLB != "vxnet-umltx6a" {
		t.Errorf("incorrect defaultVxNetForLB: %s", cfg.Global.DefaultVxNetForLB)
	}
}

func TestZones(t *testing.T) {
	qc := QingCloud{zone: "ap1"}

	z, ok := qc.Zones()
	if !ok {
		t.Fatalf("Zones() returned false")
	}

	zone, err := z.GetZone(context.TODO())
	if err != nil {
		t.Fatalf("GetZone() returned error: %s", err)
	}

	if zone.Region != qc.zone {
		t.Fatalf("GetZone() returned wrong region (%s)", zone)
	}
}

func TestCompareSpecAndLoadBalancer(t *testing.T) {

	qc := QingCloud{}

	loadBalancer, _, err := getMockupQingCloud()
	serviceEip, serviceVxNet := getMockupServiceSpec()
	if err != nil {
		t.Fatal(err)
	}

	//clusterName := "test_cluster"
	fmt.Println("-------------- Start testing of FUNC compareSpecAndLoadBalancer -----------")
	for _, lb := range loadBalancer.LoadBalancerSet {
		result, err := qc.compareSpecAndLoadBalancer(serviceEip, lb)
		if err != nil {
			t.Fatal(err)
		}
		if result != "update" {
			t.Fatalf("Compare eip data but get rong result from FUNC compareSpecAndLoadBalancer, expected is 'update'")
		}

		serviceEip.SetAnnotations(map[string]string{
			ServiceAnnotationLoadBalancerEipIds: "eip-ekre2wcl",
			ServiceAnnotationLoadBalancerType:   "0",
		})
		result, err = qc.compareSpecAndLoadBalancer(serviceEip, lb)
		if err != nil {
			t.Fatal(err)
		}
		//fmt.Println(result)
		if result != "skip" {
			t.Fatalf("Compare eip data but get wrong result from FUNC compareSpecAndLoadBalancer, expected is 'skip'")
		}
		result, err = qc.compareSpecAndLoadBalancer(serviceVxNet, lb)
		if err != nil {
			t.Fatal(err)
		}
		//fmt.Println(result)
		if result != "delete" {
			t.Fatalf("Compare vxnet data but get wrong result from FUNC compareSpecAndLoadBalancer, expected is 'delete'")
		}

		serviceVxNet.SetAnnotations(map[string]string{
			ServiceAnnotationLoadBalancerVxnetId: "vxnet-umltx6b",
			ServiceAnnotationLoadBalancerType:    "0",
		})

		*lb.VxNetID = "vxnet-umltx6b"

		result, err = qc.compareSpecAndLoadBalancer(serviceVxNet, lb)
		if err != nil {
			t.Fatal(err)
		}
		if result != "delete" {
			t.Fatalf("Compare vxnet data but get wrong result from FUNC compareSpecAndLoadBalancer, expected is 'delete'")
		}

		for _, eip := range lb.Cluster {
			*eip.EIPID = ""
		}
		result, err = qc.compareSpecAndLoadBalancer(serviceVxNet, lb)
		if err != nil {
			t.Fatal(err)
		}
		if result != "skip" {
			t.Fatalf("Compare vxnet data but get wrong result from FUNC compareSpecAndLoadBalancer, expected is 'skip'")
		}
	}
	fmt.Println("-------------- End testing of FUNC compareSpecAndLoadBalancer -----------")
	fmt.Println("-------------- Start testing of FUNC compareSpecAndLoadBalancerListeners -----------")
	_, qyLBListenersOutput, err := getMockupQingCloud()
	serviceEip, serviceVxNet = getMockupServiceSpec()
	if err != nil {
		t.Fatal(err)
	}
	qyLBListeners := []*qcservice.LoadBalancerListener{}
	qyLBListeners = append(qyLBListeners, qyLBListenersOutput.LoadBalancerListenerSet...)
	k8sTCPPorts := []int{80}
	result := qc.compareSpecAndLoadBalancerListeners(qyLBListeners, k8sTCPPorts, "roundrobin")
	if err != nil {
		t.Fatal(err)
	}
	//fmt.Println(result)
	if result != "skip" {
		t.Fatalf("Compare eip data but get rong result from FUNC compareSpecAndLoadBalancerListeners, expected is 'skip'")
	}
	result = qc.compareSpecAndLoadBalancerListeners(qyLBListeners, k8sTCPPorts, "source")
	if err != nil {
		t.Fatal(err)
	}
	//fmt.Println(result)
	if result != "update" {
		t.Fatalf("Compare eip data but get rong result from FUNC compareSpecAndLoadBalancerListeners, expected is 'update'")
	}
	k8sTCPPorts = []int{81}
	result = qc.compareSpecAndLoadBalancerListeners(qyLBListeners, k8sTCPPorts, "roundrobin")
	if err != nil {
		t.Fatal(err)
	}
	//fmt.Println(result)
	if result != "update" {
		t.Fatalf("Compare eip data but get rong result from FUNC compareSpecAndLoadBalancerListeners, expected is 'update'")
	}
	k8sTCPPorts = []int{80, 81}
	result = qc.compareSpecAndLoadBalancerListeners(qyLBListeners, k8sTCPPorts, "roundrobin")
	if err != nil {
		t.Fatal(err)
	}
	//fmt.Println(result)
	if result != "update" {
		t.Fatalf("Compare eip data but get rong result from FUNC compareSpecAndLoadBalancerListeners, expected is 'update'")
	}
	fmt.Println("-------------- End testing of FUNC compareSpecAndLoadBalancerListeners -----------")
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
