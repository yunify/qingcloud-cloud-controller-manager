package endpoint

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
)

const (
	endpointRunWorkerPeriod = 1 * time.Second
	endpointWorkers         = 1
)

type EndpointController struct {
	cloud cloudprovider.Interface

	// endpoint
	// endpointInformer     coreinformers.EndpointsInformer
	endpointLister       corelisters.EndpointsLister
	endpointListerSynced cache.InformerSynced
	// endpoints that need to be synced
	endpointQueue workqueue.RateLimitingInterface

	// svc: get service for the endpoint to get select
	serviceLister       corelisters.ServiceLister
	serviceListerSynced cache.InformerSynced

	// node: get node to filter for local service
	nodeLister       corelisters.NodeLister
	nodeListerSynced cache.InformerSynced
}

func StartEndpointControllerWrapper(completedConfig *cloudcontrollerconfig.CompletedConfig, cloud cloudprovider.Interface) cloudproviderapp.InitFunc {
	return func(ctx genericcontrollermanager.ControllerContext) (http.Handler, bool, error) {
		return startEndpointController(completedConfig, cloud, ctx.Stop)
	}
}

func startEndpointController(ctx *cloudcontrollerconfig.CompletedConfig, cloud cloudprovider.Interface, stopCh <-chan struct{}) (http.Handler, bool, error) {
	// Start the endpoint controller
	endpointController, err := New(
		cloud,
		ctx.ClientBuilder.ClientOrDie("endpoint-controller"),
		ctx.SharedInformers.Core().V1().Endpoints(),
		ctx.SharedInformers.Core().V1().Services(),
		ctx.SharedInformers.Core().V1().Nodes(),
	)
	if err != nil {
		// This error shouldn't fail. It lives like this as a legacy.
		klog.Errorf("Failed to start endpoint controller: %v", err)
		return nil, false, nil
	}

	go endpointController.Run(stopCh, endpointWorkers)

	return nil, true, nil
}

func New(
	cloud cloudprovider.Interface,
	kubeClient clientset.Interface,
	endpointInformer coreinformers.EndpointsInformer,
	serviceInformer coreinformers.ServiceInformer,
	nodeInformer coreinformers.NodeInformer,
) (*EndpointController, error) {

	ep := &EndpointController{
		cloud:                cloud,
		endpointLister:       endpointInformer.Lister(),
		endpointListerSynced: endpointInformer.Informer().HasSynced,
		endpointQueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "endpoint"),
		serviceLister:        serviceInformer.Lister(),
		serviceListerSynced:  serviceInformer.Informer().HasSynced,
		nodeLister:           nodeInformer.Lister(),
		nodeListerSynced:     nodeInformer.Informer().HasSynced,
	}

	endpointInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(old, cur interface{}) {
				_, ok1 := old.(*corev1.Endpoints)
				_, ok2 := cur.(*corev1.Endpoints)
				if ok1 && ok2 {
					ep.enqueueEndpoint(cur)
				}
			},
		},
	)

	return ep, nil
}

func (epc *EndpointController) enqueueEndpoint(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}
	epc.endpointQueue.Add(key)
}

func (epc *EndpointController) Run(stopCh <-chan struct{}, workers int) {
	defer runtime.HandleCrash()
	defer epc.endpointQueue.ShutDown()

	klog.Info("Starting endpoint controller")
	defer klog.Info("Shutting down endpoint controller")

	if !cache.WaitForCacheSync(stopCh, epc.serviceListerSynced, epc.nodeListerSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(epc.worker, endpointRunWorkerPeriod, stopCh)
	}

	<-stopCh
}

func (epc *EndpointController) worker() {
	for epc.processNextWorkItem() {
	}
}

func (epc *EndpointController) processNextWorkItem() bool {
	key, quit := epc.endpointQueue.Get()
	if quit {
		return false
	}
	defer epc.endpointQueue.Done(key)

	err := epc.handleEndpointsUpdate(key.(string))
	if err == nil {
		epc.endpointQueue.Forget(key)
		return true
	}

	runtime.HandleError(fmt.Errorf("error processing service %v (will retry): %v", key, err))
	epc.endpointQueue.AddRateLimited(key)

	return true
}

func (epc *EndpointController) handleEndpointsUpdate(key string) error {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished handleEndpointsUpdate  %q (%v)", key, time.Since(startTime))
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	// 1. get service of this endpoint to get service type and service externalTraffixPolicy
	svc, err := epc.serviceLister.Services(namespace).Get(name)
	if err != nil {
		return fmt.Errorf("get service %s/%s error: %v", namespace, name, err)
	}
	// ignore service which service type != loadbalancer or externalTrafficPolicy != Local
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer || svc.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyTypeLocal {
		klog.Infof("service %s serviceType = %s, externalTrafficPolicy = %s, skip handle endpoint update", svc.Name, svc.Spec.Type, svc.Spec.ExternalTrafficPolicy)
		return nil
	}

	// 2. get node list
	var nodes []*corev1.Node
	nodeList, err := epc.nodeLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("get node list error: %v", err)
	}
	for i, _ := range nodeList {
		if utils.NodeConditionCheck(nodeList[i]) {
			nodes = append(nodes, nodeList[i])
		}
	}

	// 3. update lb
	cloudLbIntf, _ := epc.cloud.LoadBalancer()
	err = cloudLbIntf.UpdateLoadBalancer(context.TODO(), "", svc, nodes)
	if err != nil {
		return fmt.Errorf("update loadbalancer for service %s/%s error: %v", svc.Namespace, svc.Name, err)
	}
	klog.Infof("update loadbalancer for service %s/%s success", svc.Namespace, svc.Name)

	return nil
}
