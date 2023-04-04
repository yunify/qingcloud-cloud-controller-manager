package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
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

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/qingcloud"
)

const (
	serviceRunWorkerPeriod = 1 * time.Second
	serviceWorkers         = 1
)

type ServiceController struct {
	cloud cloudprovider.Interface

	serviceLister       corelisters.ServiceLister
	serviceListerSynced cache.InformerSynced
	serviceQueue        workqueue.RateLimitingInterface
}

func StartServiceControllerWarpper(completedConfig *cloudcontrollerconfig.CompletedConfig, cloud cloudprovider.Interface) cloudproviderapp.InitFunc {
	return func(ctx genericcontrollermanager.ControllerContext) (http.Handler, bool, error) {
		return startServiceController(completedConfig, cloud, ctx.Stop)
	}
}

func startServiceController(ctx *cloudcontrollerconfig.CompletedConfig, cloud cloudprovider.Interface, stopCh <-chan struct{}) (http.Handler, bool, error) {
	// Start the endpoint controller
	serviceController, err := New(
		cloud,
		ctx.ClientBuilder.ClientOrDie("cloud-service-controller"),
		ctx.SharedInformers.Core().V1().Services(),
	)
	if err != nil {
		// This error shouldn't fail. It lives like this as a legacy.
		klog.Errorf("Failed to start cloud-service controller: %v", err)
		return nil, false, nil
	}

	go serviceController.Run(stopCh, serviceWorkers)

	return nil, true, nil
}

func New(
	cloud cloudprovider.Interface,
	kubeClient clientset.Interface,
	serviceInformer coreinformers.ServiceInformer,
) (*ServiceController, error) {

	sc := &ServiceController{
		cloud:               cloud,
		serviceQueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud-service"),
		serviceLister:       serviceInformer.Lister(),
		serviceListerSynced: serviceInformer.Informer().HasSynced,
	}

	serviceInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(old, cur interface{}) {
				oldSvc, ok1 := old.(*corev1.Service)
				newSvc, ok2 := cur.(*corev1.Service)
				if ok1 && ok2 && sc.needClean(oldSvc, newSvc) {
					sc.enqueueService(old)
				}
			},
		},
	)

	return sc, nil
}

// for service which reuse lb, need to clean listener in those situations
// change lb: lb1 -> lb2, need to clean lb1's listener ==> TODO
// change service type: loadbalancer -> nodeport/clusterip, if service changed type and delete annotation, need clean lb(auto created lb) or listener(reuse-lb)
func (sc *ServiceController) needClean(old, new *corev1.Service) bool {
	if old.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if len(old.Annotations) == 0 {
			klog.V(4).Infof("service %s/%s last config has no annotation, cloud-service-controller do nothing!", old.Namespace, old.Name)
			return false
		}
		if new.Annotations == nil {
			new.Annotations = make(map[string]string)
		}

		if new.Spec.Type == corev1.ServiceTypeLoadBalancer {
			//TODO: change lb, clean listener for old listener  if change lb
			klog.V(4).Infof("service %s/%s type not change, cloud-service-controller do nothing!", old.Namespace, old.Name)
			return false
		}

		// change service type
		// reuse-lb, clean listener if new service delete annotation
		oldStrategy := old.Annotations[qingcloud.ServiceAnnotationLoadBalancerPolicy]
		oldLBID := old.Annotations[qingcloud.ServiceAnnotationLoadBalancerID]
		newStrategy := new.Annotations[qingcloud.ServiceAnnotationLoadBalancerPolicy]
		newLBID := new.Annotations[qingcloud.ServiceAnnotationLoadBalancerID]
		if oldStrategy == qingcloud.ReuseExistingLB && oldLBID != "" {
			if newStrategy == oldStrategy && newLBID == oldLBID {
				// new service keep reuse lb annotation, ignore!
				klog.V(4).Infof("service %s/%s keep reuse lb id annotation, cloud-service-controller do nothing!", old.Namespace, old.Name)
				return false
			}
			klog.V(4).Infof("service %s/%s deleted reuse lb id annotation, cloud-service-controller will try to delete lb later!", old.Namespace, old.Name)
			return true
		}

	}
	return false
}

func (sc *ServiceController) enqueueService(obj interface{}) {
	_, ok := obj.(*corev1.Service)
	if !ok {
		return
	}
	sc.serviceQueue.Add(obj)
}

func (sc *ServiceController) Run(stopCh <-chan struct{}, workers int) {
	defer runtime.HandleCrash()
	defer sc.serviceQueue.ShutDown()

	klog.Info("Starting cloud service controller")
	defer klog.Info("Shutting down cloud service controller")

	if !cache.WaitForCacheSync(stopCh, sc.serviceListerSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(sc.worker, serviceRunWorkerPeriod, stopCh)
	}

	<-stopCh
}

func (sc *ServiceController) worker() {
	for sc.processNextWorkItem() {
	}
}

func (sc *ServiceController) processNextWorkItem() bool {
	obj, quit := sc.serviceQueue.Get()
	if quit {
		return false
	}
	defer sc.serviceQueue.Done(obj)

	svc, ok := obj.(*corev1.Service)
	if !ok {
		runtime.HandleError(fmt.Errorf("error assert service %v (will retry)", svc))
		return true
	}
	err := sc.handleServiceUpdate(svc)
	if err == nil {
		sc.serviceQueue.Forget(obj)
		return true
	}

	runtime.HandleError(fmt.Errorf("error processing service %s/%s (will retry): %v", svc.Namespace, svc.Name, err))
	sc.serviceQueue.AddRateLimited(obj)

	return true
}

func (sc *ServiceController) handleServiceUpdate(svc *corev1.Service) error {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished handleEndpointsUpdate  %s/%s (%v)", svc.Namespace, svc.Name, time.Since(startTime))
	}()

	cloudLbIntf, _ := sc.cloud.LoadBalancer()

	listenerName := fmt.Sprintf("listener_%s_%s_", svc.Namespace, svc.Name)
	lbID := svc.Annotations[qingcloud.ServiceAnnotationLoadBalancerID]

	klog.Infof("service %s/%s type changed, and loadbalancer annotation has been deleted, try to deleting listener %s of loadbalancer %s",
		svc.Namespace, svc.Name, listenerName, lbID)
	err := cloudLbIntf.EnsureLoadBalancerDeleted(context.TODO(), "", svc)
	if err != nil {
		return fmt.Errorf("delete listener %s of loadbalancer %s for service %s/%s error: %v", listenerName, lbID, svc.Namespace, svc.Name, err)
	}

	klog.Infof("delete listener %s of loadbalancer %s for service %s/%s successful", listenerName, lbID, svc.Namespace, svc.Name)
	return nil
}
