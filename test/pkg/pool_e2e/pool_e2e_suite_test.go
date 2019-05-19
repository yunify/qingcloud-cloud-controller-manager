package pool_e2e_test

import (
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/pool"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/qcapiwrapper"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/e2eutil"
	qc "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func TestPoolE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Test Suite in pool mode")
}

const (
	ControllerName = "cloud-controller-manager"
	TestZone       = "ap2a"
)

var (
	workspace     string
	testNamespace string
	k8sclient     *kubernetes.Clientset
	qcService     *qc.QingCloudService
	apiwrapper    *qcapiwrapper.QingcloudAPIWrapper
)

var _ = BeforeSuite(func() {
	testNamespace = os.Getenv("TEST_NS")
	Expect(testNamespace).ShouldNot(BeEmpty())
	workspace = e2eutil.GetWorkspace() + "/../../.."
	home := e2eutil.HomeDir()
	Expect(home).ShouldNot(BeEmpty())

	Expect(e2eutil.KubectlApply(workspace+"/test/test_cases/deployment.yaml")).ShouldNot(HaveOccurred(), "Failed to deploy pod")
	//init qcservice
	qcs, err := e2eutil.GetQingcloudService()
	Expect(err).ShouldNot(HaveOccurred(), "Failed init qc service")
	apiwrapper, err = qcapiwrapper.NewQingcloudAPIWrapper(qcs.Config, TestZone)
	Expect(err).ShouldNot(HaveOccurred(), "Failed to get apiwrapper")
	needWarmup := needWarmup()

	//read config
	c, err := clientcmd.BuildConfigFromFlags("", filepath.Join(home, ".kube", "config"))
	Expect(err).ShouldNot(HaveOccurred(), "Error in load kubeconfig")
	k8sclient = kubernetes.NewForConfigOrDie(c)

	//kubectl apply
	err = e2eutil.WaitForController(k8sclient, testNamespace, ControllerName, time.Second*10, time.Minute*2)
	Expect(err).ShouldNot(HaveOccurred(), "Failed to start controller")
	if needWarmup {
		log.Println("There is not any lb in cloud, do a warming")
		time.Sleep(time.Minute)
	}
	log.Println("Ready for testing")
})

var _ = AfterSuite(func() {
	Expect(e2eutil.KubectlDelete(workspace + "/test/manager.yaml")).ShouldNot(HaveOccurred())
	Expect(e2eutil.KubectlDelete(workspace+"/test/test_cases/deployment.yaml")).ShouldNot(HaveOccurred(), "Failed to delete endpoints")
})

func needWarmup() bool {
	lbs, err := apiwrapper.LbExec.ListByPrefix(pool.PoolLBPrefixName)
	Expect(err).ShouldNot(HaveOccurred(), "Failed to list cuurent lbs")
	if len(lbs) == 0 {
		return true
	}
	return false
}
