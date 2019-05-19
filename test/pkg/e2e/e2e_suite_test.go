package e2e_test

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
)

var _ = BeforeSuite(func() {
	//init qcservice
	qcs, err := e2eutil.GetQingcloudService()
	Expect(err).ShouldNot(HaveOccurred(), "Failed init qc service")
	qcService = qcs
	testNamespace = os.Getenv("TEST_NS")
	Expect(testNamespace).ShouldNot(BeEmpty())
	workspace = e2eutil.GetWorkspace() + "/../../.."
	home := e2eutil.HomeDir()
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
})
