package clusternode

import (
	"context"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	cloudprovider "k8s.io/cloud-provider"
	cloudproviderapp "k8s.io/cloud-provider/app"
	cloudcontrollerconfig "k8s.io/cloud-provider/app/config"
	genericcontrollermanager "k8s.io/controller-manager/app"
	"k8s.io/klog"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/controllers/utils"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/qingcloud"
)

const (
	clusterNodeRunWorkerPeriod = 1 * time.Second
	clusterNodeWorkers         = 1
)

type ClusterNodeController struct {
	cloud cloudprovider.Interface

	// svc
	serviceLister       corelisters.ServiceLister
	serviceListerSynced cache.InformerSynced

	// clusternode
	nodeLister       corelisters.NodeLister
	nodeListerSynced cache.InformerSynced
	nodeQueue        workqueue.RateLimitingInterface
}

func StartClusterNodeControllerWrapper(completedConfig *cloudcontrollerconfig.CompletedConfig, cloud cloudprovider.Interface) cloudproviderapp.InitFunc {
	return func(ctx genericcontrollermanager.ControllerContext) (http.Handler, bool, error) {
		return startClusterNodeController(completedConfig, cloud, ctx.Stop)
	}
}

func startClusterNodeController(ctx *cloudcontrollerconfig.CompletedConfig, cloud cloudprovider.Interface, stopCh <-chan struct{}) (http.Handler, bool, error) {
	// Start the endpoint controller
	clusterNodeController, err := New(
		cloud,
		ctx.ClientBuilder.ClientOrDie("clusternode-controller"),
		ctx.SharedInformers.Core().V1().Services(),
		ctx.SharedInformers.Core().V1().Nodes(),
	)
	if err != nil {
		// This error shouldn't fail. It lives like this as a legacy.
		klog.Errorf("Failed to start endpoint controller: %v", err)
		return nil, false, nil
	}

	go clusterNodeController.Run(stopCh, clusterNodeWorkers)

	return nil, true, nil
}

func New(
	cloud cloudprovider.Interface,
	kubeClient clientset.Interface,
	serviceInformer coreinformers.ServiceInformer,
	nodeInformer coreinformers.NodeInformer,
) (*ClusterNodeController, error) {

	cnc := &ClusterNodeController{
		cloud:               cloud,
		serviceLister:       serviceInformer.Lister(),
		serviceListerSynced: serviceInformer.Informer().HasSynced,
		nodeLister:          nodeInformer.Lister(),
		nodeListerSynced:    nodeInformer.Informer().HasSynced,
		nodeQueue:           workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cluster-node"),
	}

	nodeInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(old, cur interface{}) {
				oldNode, ok1 := old.(*corev1.Node)
				curNode, ok2 := cur.(*corev1.Node)
				if ok1 && ok2 && cnc.needsUpdate(oldNode, curNode) {
					cnc.enqueueNode(cur)
				}
			},
		},
	)

	return cnc, nil
}

//check if node label changed
func (cnc *ClusterNodeController) needsUpdate(old, new *corev1.Node) bool {

	if len(old.Labels) != len(new.Labels) {
		return true
	}

	for newLabelKey, newLabelValue := range new.Labels {
		oldLabelValuk, ok := old.Labels[newLabelKey]
		if !ok || newLabelValue != oldLabelValuk {
			return true
		}
	}

	return false
}

func (cnc *ClusterNodeController) enqueueNode(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}
	cnc.nodeQueue.Add(key)
}

func (cnc *ClusterNodeController) Run(stopCh <-chan struct{}, workers int) {
	defer runtime.HandleCrash()
	defer cnc.nodeQueue.ShutDown()

	klog.Info("Starting cluster node controller")
	defer klog.Info("Shutting down cluster node controller")

	if !cache.WaitForCacheSync(stopCh, cnc.serviceListerSynced, cnc.nodeListerSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(cnc.worker, clusterNodeRunWorkerPeriod, stopCh)
	}

	<-stopCh
}

func (cnc *ClusterNodeController) worker() {
	for cnc.processNextWorkItem() {
	}
}

func (cnc *ClusterNodeController) processNextWorkItem() bool {
	key, quit := cnc.nodeQueue.Get()
	if quit {
		return false
	}
	defer cnc.nodeQueue.Done(key)

	err := cnc.handleNodesUpdate(key.(string))
	if err == nil {
		cnc.nodeQueue.Forget(key)
		return true
	}

	runtime.HandleError(fmt.Errorf("error processing cluster node %v (will retry): %v", key, err))
	cnc.nodeQueue.AddRateLimited(key)

	return true
}

// handleNodesUpdate handle service backend according to node lables
func (cnc *ClusterNodeController) handleNodesUpdate(key string) error {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished handleNodesUpdate  %q (%v)", key, time.Since(startTime))
	}()

	// 1. get node list
	var nodes []*corev1.Node
	nodeList, err := cnc.nodeLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("get node list error: %v", err)
	}
	for i, _ := range nodeList {
		if utils.NodeConditionCheck(nodeList[i]) {
			nodes = append(nodes, nodeList[i])
		}
	}

	// 2. list all service
	svcs, err := cnc.serviceLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list service  error: %v", err)
	}

	// 3. filter service which externalTrafficPolicy=cluster and has annotation service.beta.kubernetes.io/qingcloud-lb-backend-label
	for _, svc := range svcs {
		_, ok := svc.Annotations[qingcloud.ServiceAnnotationBackendLabel]
		if ok && svc.Spec.Type == corev1.ServiceTypeLoadBalancer &&
			svc.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeCluster {
			klog.Infof("service %s serviceType = %s, externalTrafficPolicy = %s, also has backend label annotation , going to update loadbalancer", svc.Name, svc.Spec.Type, svc.Spec.ExternalTrafficPolicy)

			// 4. update lb
			lbInterface, _ := cnc.cloud.LoadBalancer()
			err = lbInterface.UpdateLoadBalancer(context.TODO(), "", svc, nodes)
			if err != nil {
				return fmt.Errorf("update loadbalancer for service %s/%s  error: %v", svc.Namespace, svc.Name, err)
			}
			klog.Infof("update loadbalancer for service %s/%s success", svc.Namespace, svc.Name)
		}
	}

	return nil
}
