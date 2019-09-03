package e2e_test

import (
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/e2eutil"
	qc "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}

const ControllerName = "cloud-controller-manager"

var (
	workspace     string
	testNamespace string
	k8sclient     *kubernetes.Clientset
	qcService     *qc.QingCloudService
	qingcloudLB   executor.QingCloudLoadBalancerExecutor
	testEIPID     string
	testEIPAddr   string
)

func getWorkspace() string {
	_, filename, _, _ := runtime.Caller(0)
	return path.Dir(filename)
}

var _ = BeforeSuite(func() {
	//init qcservice
	qcs, err := e2eutil.GetQingcloudService()
	Expect(err).ShouldNot(HaveOccurred(), "Failed init qc service")
	qcService = qcs
	initQingCloudLB()
	Expect(cleanup()).ShouldNot(HaveOccurred())
	testNamespace = os.Getenv("TEST_NS")
	testEIPID = os.Getenv("TEST_EIP")
	testEIPAddr = os.Getenv("TEST_EIP_ADDR")
	Expect(testNamespace).ShouldNot(BeEmpty())
	workspace = getWorkspace() + "/../../.."
	home := homeDir()
	Expect(home).ShouldNot(BeEmpty())
	//read config
	c, err := clientcmd.BuildConfigFromFlags("", filepath.Join(home, ".kube", "config"))
	Expect(err).ShouldNot(HaveOccurred(), "Error in load kubeconfig")
	k8sclient = kubernetes.NewForConfigOrDie(c)
	//kubectl apply
	err = e2eutil.WaitForController(k8sclient, testNamespace, ControllerName, time.Second*10, time.Minute*2)
	Expect(err).ShouldNot(HaveOccurred(), "Failed to start controller")
	log.Println("Ready for testing")
})

var _ = AfterSuite(func() {
	cmd := exec.Command("kubectl", "delete", "-f", workspace+"/test/manager.yaml")
	Expect(cmd.Run()).ShouldNot(HaveOccurred())
	Expect(cleanup()).ShouldNot(HaveOccurred())
})

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return ""
}

func cleanup() error {
	usedLb1, err := qingcloudLB.GetLoadBalancerByName("k8s_lb_kubernetes_" + testEIPID)
	if err != nil {
		if errors.IsResourceNotFound(err) {
			return nil
		}
		return err
	}
	log.Println("Cleanup loadbalancers")
	return qingcloudLB.Delete(*usedLb1.LoadBalancerID)
}

func initQingCloudLB() {
	lbapi, _ := qcService.LoadBalancer(qcService.Config.Zone)
	jobapi, _ := qcService.Job(qcService.Config.Zone)
	api, _ := qcService.Accesskey(qcService.Config.Zone)
	output, err := api.DescribeAccessKeys(&qc.DescribeAccessKeysInput{
		AccessKeys: []*string{&qcService.Config.AccessKeyID},
	})
	Expect(err).ShouldNot(HaveOccurred())
	qingcloudLB = executor.NewQingCloudLoadBalanceExecutor(*output.AccessKeySet[0].Owner, lbapi, jobapi, nil)
}
