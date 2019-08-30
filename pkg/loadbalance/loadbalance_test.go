package loadbalance_test

import (
	"context"

	"k8s.io/apimachinery/pkg/util/intstr"

	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/loadbalance"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/fake"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var _ = Describe("Loadbalance", func() {
	node1 := &corev1.Node{}
	node1.Name = "testnode1"
	node2 := &corev1.Node{}
	node2.Name = "testnode2"
	testService := &corev1.Service{}
	BeforeEach(func() {
		testService = &corev1.Service{}
		service := `{
			"kind": "Service",
			"apiVersion": "v1",
			"metadata": {
				"name": "mylbapp",
				"namespace": "default",
				"annotations": {
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
						"protocol": "TCP",
						"targetPort": 80,
						"nodePort": 30000
					}
				]
			}
		}`
		reader := strings.NewReader(service)
		err := yaml.NewYAMLOrJSONDecoder(reader, 10).Decode(testService)
		Expect(err).ShouldNot(HaveOccurred(), "Cannot unmarshal yamls")
		testService.SetUID(types.UID("11111-2222-3333"))
	})

	It("Should update well when nodes changed", func() {
		testService.Annotations["service.beta.kubernetes.io/qingcloud-load-balancer-eip-source"] = "auto"
		lbexec := fake.NewFakeQingCloudLBExecutor()
		sgexec := fake.NewFakeSecurityGroupExecutor()
		//add one more port
		testService.Spec.Ports = append(testService.Spec.Ports, corev1.ServicePort{
			Name:       "https",
			Port:       8443,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(443),
			NodePort:   30001,
		})
		lb, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			EipHelper:   lbexec,
			LbExecutor:  lbexec,
			SgExecutor:  sgexec,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})

		Expect(lb).ShouldNot(BeNil())
		Expect(lb.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lb.Status.K8sLoadBalancerStatus.Ingress).To(HaveLen(1))
		Expect(lbexec.Listeners).To(HaveLen(2))
		Expect(lbexec.Backends).To(HaveLen(4))

		//change both ports
		testService.Spec.Ports[0].Port = 80
		testService.Spec.Ports[1].Port = 443
		lb.TCPPorts = []int{80, 443}
		Expect(lb.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lb.Status.K8sLoadBalancerStatus.Ingress).To(HaveLen(1))
		Expect(lbexec.Backends).To(HaveLen(4))
		Expect(lbexec.Listeners).To(HaveLen(2))

		lb.Nodes = make([]*corev1.Node, 0)
		Expect(lb.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lbexec.Backends).To(HaveLen(0))
		lb.Nodes = append(lb.Nodes, node1, node2)
		Expect(lb.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lbexec.Backends).To(HaveLen(4))
	})

	It("Should delete old listeners when changing service ports", func() {
		testService.Annotations["service.beta.kubernetes.io/qingcloud-load-balancer-eip-source"] = "auto"
		lbexec := fake.NewFakeQingCloudLBExecutor()
		sgexec := fake.NewFakeSecurityGroupExecutor()
		lb, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			EipHelper:   lbexec,
			LbExecutor:  lbexec,
			SgExecutor:  sgexec,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})
		Expect(lb).ShouldNot(BeNil())
		Expect(lb.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lb.Status.K8sLoadBalancerStatus.Ingress).To(HaveLen(1))
		testService.Spec.Ports[0].Port = 8089
		lb.TCPPorts = []int{8089}
		Expect(lb.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		for _, lib := range lbexec.Listeners {
			Expect(*lib.ListenerPort).To(Equal(8089))
		}
	})

	It("Should auto get an ip if we use auto mode", func() {
		testService.Annotations["service.beta.kubernetes.io/qingcloud-load-balancer-eip-source"] = "auto"
		lbexec := fake.NewFakeQingCloudLBExecutor()
		sgexec := fake.NewFakeSecurityGroupExecutor()
		lb, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			EipHelper:   lbexec,
			LbExecutor:  lbexec,
			SgExecutor:  sgexec,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})
		Expect(lb).ShouldNot(BeNil())
		Expect(lb.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lb.Status.K8sLoadBalancerStatus.Ingress).To(HaveLen(1))
		Expect(lbexec.ReponseEIPs).To(HaveLen(1))
		for _, k := range lbexec.ReponseEIPs {
			Expect(lb.Status.K8sLoadBalancerStatus.Ingress[0].IP).To(Equal(k.Address))
			Expect(k.Status).To(Equal("allocate"))
		}
		Expect(lb.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		for _, lib := range lbexec.Listeners {
			Expect(*lib.ListenerPort).To(Equal(8088))
		}
		Expect(lb.DeleteQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lbexec.LoadBalancers).To(HaveLen(0))
		Expect(sgexec.SecurityGroups).To(HaveLen(0))
		Expect(lbexec.ReponseEIPs).To(HaveLen(0))
	})
	It("Should use avaliable ip", func() {
		testService.Annotations["service.beta.kubernetes.io/qingcloud-load-balancer-eip-source"] = "use-available"
		lbexec := fake.NewFakeQingCloudLBExecutor()
		sgexec := fake.NewFakeSecurityGroupExecutor()
		dip := lbexec.AddEIP("eip-vmldumvv", "1.1.1.1")
		lb, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			EipHelper:   lbexec,
			LbExecutor:  lbexec,
			SgExecutor:  sgexec,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})
		Expect(lb).ShouldNot(BeNil())
		Expect(lb.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		for _, lib := range lbexec.Listeners {
			Expect(*lib.ListenerPort).To(Equal(8088))
		}
		Expect(lb.Status.K8sLoadBalancerStatus.Ingress).To(HaveLen(1))
		Expect(lbexec.ReponseEIPs).To(HaveLen(1))
		Expect(lb.Status.K8sLoadBalancerStatus.Ingress[0].IP).To(Equal(dip.Address))
		Expect(lb.DeleteQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lbexec.LoadBalancers).To(HaveLen(0))
		Expect(sgexec.SecurityGroups).To(HaveLen(0))
		Expect(lbexec.ReponseEIPs).To(HaveLen(1))
	})
	It("Should follow the topology", func() {
		testService.Annotations["service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids"] = "eip-vmldumvv"
		lbexec := fake.NewFakeQingCloudLBExecutor()
		dip := lbexec.AddEIP("eip-vmldumvv", "1.1.1.1")
		sgexec := fake.NewFakeSecurityGroupExecutor()
		lb, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			EipHelper:   lbexec,
			LbExecutor:  lbexec,
			SgExecutor:  sgexec,
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
		Expect(lb.CreateQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(*lb.Status.QcLoadBalancer.LoadBalancerType).To(Equal(0))
		Expect(lb.Status.K8sLoadBalancerStatus.Ingress[0].IP).Should(Equal(dip.Address))
		Expect(lbexec.Backends).To(HaveLen(2))
		defer func() {
			Expect(lb.DeleteQingCloudLB()).ShouldNot(HaveOccurred())
			Expect(lbexec.LoadBalancers).To(HaveLen(0))
			Expect(sgexec.SecurityGroups).To(HaveLen(0))
			Expect(lbexec.ReponseEIPs).To(HaveLen(1))
		}()
		//Change Service type

		lb.GetService().Annotations["service.beta.kubernetes.io/qingcloud-load-balancer-type"] = "1"
		lb.Type = 1
		Expect(lb.NeedResize()).To(BeTrue())
		Expect(lb.UpdateQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(*lb.Status.QcLoadBalancer.LoadBalancerType).To(Equal(1))

		//add node
		node3 := &corev1.Node{}
		node3.Name = "testnode3"
		lb.Nodes = append(lb.Nodes, node3)
		Expect(lb.UpdateQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lbexec.Backends).To(HaveLen(3))

		//delete node
		lb.Nodes = lb.Nodes[:2]
		Expect(lb.UpdateQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lbexec.Backends).To(HaveLen(2))
	})
	It("Should be ok when in reuse mode", func() {
		testService1 := &corev1.Service{}
		service := `{
			"kind": "Service",
			"apiVersion": "v1",
			"metadata": {
				"name": "mylbapp",
				"namespace": "default",
				"annotations": {
					"service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids": "eip-vmldumvv",
					"service.beta.kubernetes.io/qingcloud-load-balancer-type": "0",
					"service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy": "reuse"
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
						"protocol": "TCP",
						"nodePort": 30000
					}
				]
			}
		}`
		reader := strings.NewReader(service)
		err := yaml.NewYAMLOrJSONDecoder(reader, 10).Decode(testService1)
		Expect(err).ShouldNot(HaveOccurred(), "Cannot unmarshal yamls")
		testService1.SetUID(types.UID("11111-2222-3333"))

		testService2 := &corev1.Service{}
		service = `{
			"kind": "Service",
			"apiVersion": "v1",
			"metadata": {
				"name": "mylbapp2",
				"namespace": "default",
				"annotations": {
					"service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids": "eip-vmldumvv",
					"service.beta.kubernetes.io/qingcloud-load-balancer-type": "0",
					"service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy": "reuse"
				}
			},
			"spec": {
				"selector": {
					"app": "mylbapp2"
				},
				"type": "LoadBalancer",
				"ports": [
					{
						"name": "http",
						"port": 8089,
						"protocol": "TCP",
						"targetPort": 80,
						"nodePort": 30001
					}
				]
			}
		}`
		reader = strings.NewReader(service)
		err = yaml.NewYAMLOrJSONDecoder(reader, 10).Decode(testService2)
		Expect(err).ShouldNot(HaveOccurred(), "Cannot unmarshal yamls")
		testService2.SetUID(types.UID("11111-2222-3333-444"))
		lbexec := fake.NewFakeQingCloudLBExecutor()
		dip := lbexec.AddEIP("eip-vmldumvv", "1.1.1.1")
		sgexec := fake.NewFakeSecurityGroupExecutor()
		lb1, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService1,
			EipHelper:   lbexec,
			LbExecutor:  lbexec,
			SgExecutor:  sgexec,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})
		Expect(lb1).ShouldNot(BeNil())

		lb2, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService2,
			EipHelper:   lbexec,
			LbExecutor:  lbexec,
			SgExecutor:  sgexec,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})
		Expect(lb2).ShouldNot(BeNil())
		Expect(lb1.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lb2.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lb1.Status.K8sLoadBalancerStatus.Ingress[0].IP).Should(Equal(dip.Address))
		Expect(lb2.Status.K8sLoadBalancerStatus.Ingress[0].IP).Should(Equal(dip.Address))
		Expect(lbexec.LoadBalancers).To(HaveLen(1))
		Expect(lbexec.Listeners).To(HaveLen(2))
		Expect(lbexec.Backends).To(HaveLen(4))
		Expect(lb1.DeleteQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lbexec.LoadBalancers).To(HaveLen(1))
		Expect(sgexec.SecurityGroups).To(HaveLen(1))
		Expect(lbexec.ReponseEIPs).To(HaveLen(1))
		Expect(lbexec.Backends).To(HaveLen(2))
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
				"protocol": "TCP",
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
				"protocol": "TCP",
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
		lb, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})
		Expect(lb).ShouldNot(BeNil())
		Expect(lb.TCPPorts).To(HaveLen(3))
		Expect(lb.LoadListeners()).ShouldNot(HaveOccurred())
		Expect(lb.GetListeners()[0].Protocol).To(Equal("http"))
		Expect(lb.GetListeners()[1].Protocol).To(Equal("udp"))
	})
})
