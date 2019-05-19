package loadbalance_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/loadbalance"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/qcapiwrapper"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/e2eutil"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/fake"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Loadbalance", func() {
	node1 := &corev1.Node{}
	node1.Name = "testnode1"
	node2 := &corev1.Node{}
	node2.Name = "testnode2"
	testService := &corev1.Service{}
	var lbexec *fake.FakeQingCloudLBExecutor
	var sgexec *fake.FakeSecurityGroupExecutor
	var apiwrapper *qcapiwrapper.QingcloudAPIWrapper

	BeforeEach(func() {
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
		testService, err := e2eutil.StringToService(service)
		Expect(err).ShouldNot(HaveOccurred(), "Cannot unmarshal yamls")
		testService.SetUID(types.UID("11111-2222-3333"))
		lbexec = fake.NewFakeQingCloudLBExecutor()
		sgexec = fake.NewFakeSecurityGroupExecutor()
		apiwrapper = &qcapiwrapper.QingcloudAPIWrapper{
			EipHelper: lbexec,
			LbExec:    lbexec,
			SgExec:    sgexec,
		}
	})

	It("Should auto get an ip if we use auto mode", func() {
		testService.Annotations["service.beta.kubernetes.io/qingcloud-load-balancer-eip-source"] = "auto"
		lb, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			QcAPI:       apiwrapper,
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
		Expect(lb.DeleteQingCloudLB()).ShouldNot(HaveOccurred())
		Expect(lbexec.LoadBalancers).To(HaveLen(0))
		Expect(sgexec.SecurityGroups).To(HaveLen(0))
		Expect(lbexec.ReponseEIPs).To(HaveLen(0))
	})
	It("Should use avaliable ip", func() {
		testService.Annotations["service.beta.kubernetes.io/qingcloud-load-balancer-eip-source"] = "use-available"
		dip := lbexec.AddEIP("eip-vmldumvv", "1.1.1.1")
		lb, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			QcAPI:       apiwrapper,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})
		Expect(lb).ShouldNot(BeNil())
		Expect(lb.EnsureQingCloudLB()).ShouldNot(HaveOccurred())
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
		dip := lbexec.AddEIP("eip-vmldumvv", "1.1.1.1")
		lb, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			QcAPI:       apiwrapper,
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
		testService1, err := e2eutil.StringToService(service)
		Expect(err).ShouldNot(HaveOccurred(), "Cannot unmarshal yamls")
		testService1.SetUID(types.UID("11111-2222-3333"))

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
		testService2, err := e2eutil.StringToService(service)
		Expect(err).ShouldNot(HaveOccurred(), "Cannot unmarshal yamls")
		testService2.SetUID(types.UID("11111-2222-3333-444"))
		dip := lbexec.AddEIP("eip-vmldumvv", "1.1.1.1")
		lb1, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService1,
			QcAPI:       apiwrapper,
			ClusterName: "Test",
			Context:     context.TODO(),
			K8sNodes:    []*corev1.Node{node1, node2},
			NodeLister:  &fake.FakeNodeLister{},
		})
		Expect(lb1).ShouldNot(BeNil())

		lb2, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService2,
			QcAPI:       apiwrapper,
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
		testService, err := e2eutil.StringToService(service)
		Expect(err).ShouldNot(HaveOccurred(), "Cannot unmarshal yamls")
		testService.SetUID(types.UID("11111-2222-3333"))
		lb, _ := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			ClusterName: "Test",
			QcAPI:       apiwrapper,
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
