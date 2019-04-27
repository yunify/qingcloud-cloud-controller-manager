package loadbalance_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/loadbalance"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/fake"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"strings"
)

var _ = Describe("Loadbalance", func() {
	node1 := &corev1.Node{}
	node1.Name = "testnode1"
	node2 := &corev1.Node{}
	node2.Name = "testnode2"
	It("Should follow the topology", func() {
		testService := &corev1.Service{}
		service := `
kind: Service
apiVersion: v1
metadata:
  name:  mylbapp
  namespace: default
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids: "eip-vmldumvv"
    service.beta.kubernetes.io/qingcloud-load-balancer-type: "0"
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer 
  ports:
  - name:  http
    port:  8088
    targetPort:  80
    nodePort: 30000`
		reader := strings.NewReader(service)
		err := yaml.NewYAMLOrJSONDecoder(reader, 10).Decode(testService)
		Expect(err).ShouldNot(HaveOccurred(), "Cannot unmarshal yamls")
		testService.SetUID(types.UID("11111-2222-3333"))
		lb := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})
		Expect(lb).ShouldNot(BeNil())
		Expect(lb.Name).To(Equal("k8s_lb_Test_mylbapp_11111"))
		Expect(lb.LoadListeners()).ShouldNot(HaveOccurred())
		Expect(lb.GetListeners()).To(HaveLen(1))
		listener := lb.GetListeners()[0]
		Expect(listener.Name).To(Equal("listener_default_mylbapp_8088"))
		Expect(listener.ListenerPort).To(Equal(8088))
		Expect(listener.Protocol).To(Equal("http"))
		listener.LoadBackends()
		backends := listener.GetBackends()
		Expect(backends.Items).To(HaveLen(2))
		backend := backends.Items[0]
		expected := &loadbalance.Backend{
			Name: "backend_" + listener.Name + "_testnode1",
			Spec: loadbalance.BackendSpec{
				Listener:   listener,
				Weight:     1,
				Port:       30000,
				InstanceID: "testnode1",
			},
		}
		Expect(backend).To(Equal(expected))
	})

	It("Should treat port name well", func() {
		testService := &corev1.Service{}
		service := `{
	"kind": "Service",
	"apiVersion": "v1",
	"metadata": {
		"name": "mylbapp",
		"namespace": "default",
		"annotations": {
			"service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids": "eip-vmldumvv",
			"service.beta.kubernetes.io/qingcloud-load-balancer-type": "0"
		}
	},
	"spec": {
		"selector": {
			"app": "mylbapp"
		},
		"type": "LoadBalancer",
		"ports": [
			{
				"name": "http",
				"port": 8088,
				"targetPort": 80,
				"nodePort": 30000
			},
			{
				"name": "udp",
				"protocol": "UDP",
				"port": 8089,
				"targetPort": 81,
				"nodePort": 30001
			},
			{
				"name": "test",
				"port": 8090,
				"targetPort": 82,
				"nodePort": 30002
			}
		]
	}
}`
		reader := strings.NewReader(service)
		err := yaml.NewYAMLOrJSONDecoder(reader, 10).Decode(testService)
		Expect(err).ShouldNot(HaveOccurred(), "Cannot unmarshal yamls")
		testService.SetUID(types.UID("11111-2222-3333"))
		lb := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})
		Expect(lb).ShouldNot(BeNil())
		Expect(lb.TCPPorts).To(HaveLen(2))
		Expect(lb.LoadListeners()).ShouldNot(HaveOccurred())
		Expect(lb.GetListeners()[0].Protocol).To(Equal("http"))
		Expect(lb.GetListeners()[1].Protocol).To(Equal("tcp"))
	})
})
