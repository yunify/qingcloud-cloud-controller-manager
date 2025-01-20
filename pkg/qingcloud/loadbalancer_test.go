package qingcloud

import (
	"reflect"
	"testing"

	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
)

func TestConvertLoadBalancerStatus(t *testing.T) {
	testCases := []*apis.LoadBalancerStatus{
		{
			VIP: []string{"139.198.1.1"},
		},
	}

	for _, tc := range testCases {
		status := convertLoadBalancerStatus(tc)
		if len(status.Ingress) != len(tc.VIP) {
			t.Errorf("covertLoadBalancerStatus Error, Ingress ip not correct")
		}
	}
}

func TestNeedUpdateAttr(t *testing.T) {
	testLoadBalancerID := "testLB"

	testCases := []struct {
		update *apis.LoadBalancer
		config *LoadBalancerConfig
		status *apis.LoadBalancer
	}{
		{
			update: nil,
			config: &LoadBalancerConfig{
				NodeCount:  nil,
				InternalIP: nil,
			},
			status: &apis.LoadBalancer{
				Spec: apis.LoadBalancerSpec{
					NodeCount:  qcservice.Int(2),
					PrivateIPs: []*string{qcservice.String("192.168.1.1")},
				},
				Status: apis.LoadBalancerStatus{
					LoadBalancerID: &testLoadBalancerID,
				},
			},
		},
		{
			update: &apis.LoadBalancer{
				Spec: apis.LoadBalancerSpec{
					NodeCount: qcservice.Int(1),
				},
				Status: apis.LoadBalancerStatus{
					LoadBalancerID: &testLoadBalancerID,
				},
			},
			config: &LoadBalancerConfig{
				NodeCount:  qcservice.Int(1),
				InternalIP: nil,
			},
			status: &apis.LoadBalancer{
				Spec: apis.LoadBalancerSpec{
					NodeCount:  qcservice.Int(2),
					PrivateIPs: []*string{qcservice.String("192.168.1.1")},
				},
				Status: apis.LoadBalancerStatus{
					LoadBalancerID: &testLoadBalancerID,
				},
			},
		},
		{
			update: &apis.LoadBalancer{
				Spec: apis.LoadBalancerSpec{
					PrivateIPs: []*string{qcservice.String("192.168.1.2")},
				},
				Status: apis.LoadBalancerStatus{
					LoadBalancerID: &testLoadBalancerID,
				},
			},
			config: &LoadBalancerConfig{
				NodeCount:  nil,
				InternalIP: qcservice.String("192.168.1.2"),
			},
			status: &apis.LoadBalancer{
				Spec: apis.LoadBalancerSpec{
					NodeCount:  qcservice.Int(2),
					PrivateIPs: []*string{qcservice.String("192.168.1.1")},
				},
				Status: apis.LoadBalancerStatus{
					LoadBalancerID: &testLoadBalancerID,
				},
			},
		},
		{
			update: &apis.LoadBalancer{
				Spec: apis.LoadBalancerSpec{
					NodeCount:  qcservice.Int(1),
					PrivateIPs: []*string{qcservice.String("192.168.1.2")},
				},
				Status: apis.LoadBalancerStatus{
					LoadBalancerID: &testLoadBalancerID,
				},
			},
			config: &LoadBalancerConfig{
				NodeCount:  qcservice.Int(1),
				InternalIP: qcservice.String("192.168.1.2"),
			},
			status: &apis.LoadBalancer{
				Spec: apis.LoadBalancerSpec{
					NodeCount:  qcservice.Int(2),
					PrivateIPs: []*string{qcservice.String("192.168.1.1")},
				},
				Status: apis.LoadBalancerStatus{
					LoadBalancerID: &testLoadBalancerID,
				},
			},
		},
	}

	for _, tc := range testCases {
		result := needUpdateAttr(tc.config, tc.status)
		if !reflect.DeepEqual(result, tc.update) {
			t.Fail()
		}
	}
}

func TestDiffListeners(t *testing.T) {
	testCases := []struct {
		listeners []*apis.LoadBalancerListener
		ports     []v1.ServicePort
		toDelete  []*string
		toAdd     []v1.ServicePort
		conf      *LoadBalancerConfig
	}{
		{
			listeners: []*apis.LoadBalancerListener{
				{
					Spec: apis.LoadBalancerListenerSpec{
						ListenerPort:       qcservice.Int(8080),
						ListenerProtocol:   qcservice.String("tcp"),
						HealthyCheckMethod: qcservice.String("tcp"),
						HealthyCheckOption: qcservice.String(defaultListenerHeathyCheckOption),
						BalanceMode:        qcservice.String(defaultListenerBalanceMode),
						Timeout:            qcservice.Int(defaultTimeout),
						Scene:              qcservice.Int(defaultScene),
						Forwardfor:         qcservice.Int(defaultForwardfor),
						ListenerOption:     qcservice.Int(defaultListenerOption),
					},
					Status: apis.LoadBalancerListenerStatus{
						LoadBalancerListenerID: qcservice.String("testListener"),
						LoadBalancerBackends: []*apis.LoadBalancerBackend{
							{
								Spec: apis.LoadBalancerBackendSpec{
									LoadBalancerBackendName: qcservice.String("instance1"),
									Port:                    qcservice.Int(9090),
									ResourceID:              qcservice.String("instance1"),
								},
								Status: apis.LoadBalancerBackendStatus{
									LoadBalancerBackendID: qcservice.String("testBackend"),
								},
							},
						},
					},
				},
			},
			ports: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     8080,
					NodePort: 9090,
				},
			},
			toAdd:    nil,
			toDelete: nil,
		},
		{
			listeners: []*apis.LoadBalancerListener{
				{
					Spec: apis.LoadBalancerListenerSpec{
						ListenerPort:       qcservice.Int(8080),
						ListenerProtocol:   qcservice.String("tcp"),
						HealthyCheckMethod: qcservice.String("tcp"),
						HealthyCheckOption: qcservice.String("10|5|2|5"),
						BalanceMode:        qcservice.String("roundrobin"),
						Timeout:            qcservice.Int(50),
						Scene:              qcservice.Int(0),
						Forwardfor:         qcservice.Int(0),
						ListenerOption:     qcservice.Int(0),
					},
					Status: apis.LoadBalancerListenerStatus{
						LoadBalancerListenerID: qcservice.String("testListener"),
						LoadBalancerBackends: []*apis.LoadBalancerBackend{
							{
								Spec: apis.LoadBalancerBackendSpec{
									LoadBalancerBackendName: qcservice.String("instance1"),
									Port:                    qcservice.Int(9090),
									ResourceID:              qcservice.String("instance1"),
								},
								Status: apis.LoadBalancerBackendStatus{
									LoadBalancerBackendID: qcservice.String("testBackend"),
								},
							},
						},
					},
				},
			},
			ports: []v1.ServicePort{
				{
					Protocol: v1.ProtocolUDP,
					Port:     8080,
					NodePort: 9090,
				},
			},
			toAdd: []v1.ServicePort{
				{
					Protocol: v1.ProtocolUDP,
					Port:     8080,
					NodePort: 9090,
				},
			},
			toDelete: []*string{qcservice.String("testListener")},
		},
		{
			listeners: []*apis.LoadBalancerListener{
				{
					Spec: apis.LoadBalancerListenerSpec{
						ListenerPort:       qcservice.Int(8080),
						ListenerProtocol:   qcservice.String("tcp"),
						HealthyCheckMethod: qcservice.String("tcp"),
						HealthyCheckOption: qcservice.String("10|5|2|5"),
						BalanceMode:        qcservice.String("roundrobin"),
						Timeout:            qcservice.Int(50),
						Scene:              qcservice.Int(0),
						Forwardfor:         qcservice.Int(0),
						ListenerOption:     qcservice.Int(0),
					},
					Status: apis.LoadBalancerListenerStatus{
						LoadBalancerListenerID: qcservice.String("testListener"),
						LoadBalancerBackends: []*apis.LoadBalancerBackend{
							{
								Spec: apis.LoadBalancerBackendSpec{
									LoadBalancerBackendName: qcservice.String("instance1"),
									Port:                    qcservice.Int(9090),
									ResourceID:              qcservice.String("instance1"),
								},
								Status: apis.LoadBalancerBackendStatus{
									LoadBalancerBackendID: qcservice.String("testBackend"),
								},
							},
						},
					},
				},
			},
			ports: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     8081,
					NodePort: 9090,
				},
			},
			toAdd: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     8081,
					NodePort: 9090,
				},
			},
			toDelete: []*string{qcservice.String("testListener")},
		},
		{
			listeners: []*apis.LoadBalancerListener{
				{
					Spec: apis.LoadBalancerListenerSpec{
						ListenerPort:       qcservice.Int(8080),
						ListenerProtocol:   qcservice.String("tcp"),
						HealthyCheckMethod: qcservice.String("tcp"),
						HealthyCheckOption: qcservice.String("10|5|2|5"),
						BalanceMode:        qcservice.String("roundrobin"),
						Timeout:            qcservice.Int(50),
						Scene:              qcservice.Int(0),
						Forwardfor:         qcservice.Int(0),
						ListenerOption:     qcservice.Int(0),
					},
					Status: apis.LoadBalancerListenerStatus{
						LoadBalancerListenerID: qcservice.String("testListener"),
						LoadBalancerBackends: []*apis.LoadBalancerBackend{
							{
								Spec: apis.LoadBalancerBackendSpec{
									LoadBalancerBackendName: qcservice.String("instance1"),
									Port:                    qcservice.Int(9090),
									ResourceID:              qcservice.String("instance1"),
								},
								Status: apis.LoadBalancerBackendStatus{
									LoadBalancerBackendID: qcservice.String("testBackend"),
								},
							},
						},
					},
				},
			},
			ports: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     8080,
					NodePort: 9091,
				},
			},
			toAdd: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     8080,
					NodePort: 9091,
				},
			},
			toDelete: []*string{qcservice.String("testListener")},
		},
		{
			listeners: []*apis.LoadBalancerListener{
				{
					Spec: apis.LoadBalancerListenerSpec{
						ListenerPort:       qcservice.Int(8080),
						ListenerProtocol:   qcservice.String("tcp"),
						HealthyCheckMethod: qcservice.String("tcp"),
						HealthyCheckOption: qcservice.String("10|5|2|5"),
						BalanceMode:        qcservice.String("roundrobin"),
						Timeout:            qcservice.Int(50),
						Scene:              qcservice.Int(0),
						Forwardfor:         qcservice.Int(0),
						ListenerOption:     qcservice.Int(0),
					},
					Status: apis.LoadBalancerListenerStatus{
						LoadBalancerListenerID: qcservice.String("testListenerBalanceMode"),
						LoadBalancerBackends: []*apis.LoadBalancerBackend{
							{
								Spec: apis.LoadBalancerBackendSpec{
									LoadBalancerBackendName: qcservice.String("instance1"),
									Port:                    qcservice.Int(9090),
									ResourceID:              qcservice.String("instance1"),
								},
								Status: apis.LoadBalancerBackendStatus{
									LoadBalancerBackendID: qcservice.String("testBackend"),
								},
							},
						},
					},
				},
			},
			ports: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     8080,
					NodePort: 9090,
				},
			},
			conf: &LoadBalancerConfig{
				balanceMode: qcservice.String("8080:source"), // change balanceMode
			},
			toAdd: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     8080,
					NodePort: 9090,
				},
			},
			toDelete: []*string{qcservice.String("testListenerBalanceMode")},
		},
		{
			listeners: []*apis.LoadBalancerListener{
				{
					Spec: apis.LoadBalancerListenerSpec{
						ListenerPort:       qcservice.Int(8080),
						ListenerProtocol:   qcservice.String("tcp"),
						HealthyCheckMethod: qcservice.String("tcp"),
						HealthyCheckOption: qcservice.String("10|5|2|5"),
						BalanceMode:        qcservice.String("roundrobin"),
						Timeout:            qcservice.Int(50),
						Scene:              qcservice.Int(0),
						Forwardfor:         qcservice.Int(0),
						ListenerOption:     qcservice.Int(0),
					},
					Status: apis.LoadBalancerListenerStatus{
						LoadBalancerListenerID: qcservice.String("testListenerProtocol"),
						LoadBalancerBackends: []*apis.LoadBalancerBackend{
							{
								Spec: apis.LoadBalancerBackendSpec{
									LoadBalancerBackendName: qcservice.String("instance1"),
									Port:                    qcservice.Int(9090),
									ResourceID:              qcservice.String("instance1"),
								},
								Status: apis.LoadBalancerBackendStatus{
									LoadBalancerBackendID: qcservice.String("testBackend"),
								},
							},
						},
					},
				},
			},
			ports: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     8080,
					NodePort: 9090,
				},
			},
			conf: &LoadBalancerConfig{
				Protocol:          qcservice.String("8080:https"),       //change protocol
				ServerCertificate: qcservice.String("8080:sc-llluxekm"), // change cert
			},
			toAdd: []v1.ServicePort{
				{
					Protocol: v1.ProtocolTCP,
					Port:     8080,
					NodePort: 9090,
				},
			},
			toDelete: []*string{qcservice.String("testListenerProtocol")},
		},
	}

	for _, tc := range testCases {
		toDelete, toAdd, _ := diffListeners(tc.listeners, tc.conf, tc.ports)
		// fmt.Printf("delete=%s, add=%s, keep=%s\n",spew.Sdump(toDelete), spew.Sdump(toAdd), spew.Sdump(toKeep))
		if !reflect.DeepEqual(toDelete, tc.toDelete) || !reflect.DeepEqual(toAdd, tc.toAdd) {
			t.Fail()
		}
	}
}

func TestDiffBackends(t *testing.T) {
	testCases := []struct {
		listener *apis.LoadBalancerListener
		nodes    []*v1.Node
		toDelete []*string
		toAdd    []*v1.Node
	}{
		{
			listener: &apis.LoadBalancerListener{
				Status: apis.LoadBalancerListenerStatus{
					LoadBalancerBackends: []*apis.LoadBalancerBackend{
						{
							Spec: apis.LoadBalancerBackendSpec{
								LoadBalancerBackendName: qcservice.String("instance1"),
							},
							Status: apis.LoadBalancerBackendStatus{
								LoadBalancerBackendID: qcservice.String("instance1"),
							},
						},
					},
				},
			},
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "instance2",
					},
				},
			},
			toDelete: []*string{qcservice.String("instance1")},
			toAdd: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "instance2",
					},
				},
			},
		},
		{
			listener: &apis.LoadBalancerListener{
				Status: apis.LoadBalancerListenerStatus{
					LoadBalancerBackends: []*apis.LoadBalancerBackend{
						{
							Spec: apis.LoadBalancerBackendSpec{
								LoadBalancerBackendName: qcservice.String("instance1"),
							},
							Status: apis.LoadBalancerBackendStatus{
								LoadBalancerBackendID: qcservice.String("instance1"),
							},
						},
					},
				},
			},
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xxxx",
						Annotations: map[string]string{
							NodeAnnotationInstanceID: "instance2",
						},
					},
				},
			},
			toDelete: []*string{qcservice.String("instance1")},
			toAdd: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xxxx",
						Annotations: map[string]string{
							NodeAnnotationInstanceID: "instance2",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		toDelete, toAdd := diffBackend(tc.listener, tc.nodes)
		//fmt.Printf("delete=%s, add=%s", spew.Sdump(toDelete), spew.Sdump(toAdd))
		if !reflect.DeepEqual(toDelete, tc.toDelete) || !reflect.DeepEqual(toAdd, tc.toAdd) {
			t.Fail()
		}
	}
}
