// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package qingcloud

// Please see qingcloud document: https://docs.qingcloud.com/index.html
// and must pay attention to your account resource quota limit.

import (
	"context"
	"fmt"
	"io"

	"github.com/davecgh/go-spew/spew"
	yaml "gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	corev1informer "k8s.io/client-go/informers/core/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
)

const (
	ProviderName = "qingcloud"
	QYConfigPath = "/etc/qingcloud/config.yaml"
)

type Config struct {
	Zone              string   `yaml:"zone"`
	DefaultVxNetForLB string   `yaml:"defaultVxNetForLB,omitempty"`
	ClusterID         string   `yaml:"clusterID"`
	IsApp             bool     `yaml:"isApp,omitempty"`
	TagIDs            []string `yaml:"tagIDs,omitempty"`
	InstanceIDs       []string `yaml:"instanceIDs,omitempty"`
}

// A single Kubernetes cluster can run in multiple zones,
// but only within the same region (and cloud provider).

// QingCloud is the main entry of all interface
type QingCloud struct {
	Config *Config
	Client executor.QingCloudClientInterface

	nodeInformer     corev1informer.NodeInformer
	serviceInformer  corev1informer.ServiceInformer
	endpointInformer corev1informer.EndpointsInformer
	corev1interface  corev1.CoreV1Interface
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		return NewQingCloud(config)
	})
}

// NewQingCloud returns a new instance of QingCloud cloud provider.
func NewQingCloud(cfg io.Reader) (cloudprovider.Interface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no qingcloud provider Config file given")
	}

	var (
		config Config
	)
	err := yaml.NewDecoder(cfg).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Config file, err=%v", err)
	}

	client, err := executor.NewQingCloudClient(&executor.ClientConfig{
		IsAPP:  config.IsApp,
		TagIDs: config.TagIDs,
	}, QYConfigPath)
	if err != nil {
		return nil, err
	}

	qc := QingCloud{
		Config: &config,
		Client: client,
	}

	return &qc, nil
}

func (qc *QingCloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	clientset := clientBuilder.ClientOrDie("do-shared-informers")
	sharedInformer := informers.NewSharedInformerFactory(clientset, 0)
	nodeinformer := sharedInformer.Core().V1().Nodes()
	go nodeinformer.Informer().Run(stop)
	qc.nodeInformer = nodeinformer

	serviceInformer := sharedInformer.Core().V1().Services()
	go serviceInformer.Informer().Run(stop)
	qc.serviceInformer = serviceInformer

	endpointInformer := sharedInformer.Core().V1().Endpoints()
	go endpointInformer.Informer().Run(stop)
	qc.endpointInformer = endpointInformer

	qc.corev1interface = clientset.CoreV1()
}

func (qc *QingCloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

func (qc *QingCloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

func (qc *QingCloud) ProviderName() string {
	return ProviderName
}

func (qc *QingCloud) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

func (qc *QingCloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return nil, false
}

func (qc *QingCloud) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

func (qc *QingCloud) HasClusterID() bool {
	return qc.Config.ClusterID != ""
}

// LoadBalancer returns an implementation of LoadBalancer for QingCloud.
func (qc *QingCloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return qc, true
}

// GetLoadBalancerName returns the name of the load balancer. Implementations must treat the
// *v1.Service parameter as read-only and not modify it.
func (qc *QingCloud) GetLoadBalancerName(_ context.Context, _ string, service *v1.Service) string {
	return ""
}

// GetLoadBalancer returns whether the specified load balancer exists, and
// if so, what its status is.
func (qc *QingCloud) GetLoadBalancer(ctx context.Context, _ string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	_, lb, err := qc.getLoadBalancer(service)

	if errors.IsResourceNotFound(err) {
		return nil, false, nil
	}

	if err == nil {
		status = convertLoadBalancerStatus(&lb.Status)
		exists = true
	}

	return status, exists, err
}

func (qc *QingCloud) getLoadBalancer(service *v1.Service) (*LoadBalancerConfig, *apis.LoadBalancer, error) {
	var (
		lb   *apis.LoadBalancer
		err  error
		conf *LoadBalancerConfig
	)

	conf, err = qc.ParseServiceLBConfig(qc.Config.ClusterID, service)
	if err != nil {
		return nil, nil, err
	}

	if conf.ReuseLBID != "" {
		lb, err = qc.Client.GetLoadBalancerByID(conf.ReuseLBID)
	} else if conf.LoadBalancerName != "" {
		lb, err = qc.Client.GetLoadBalancerByName(conf.LoadBalancerName)
	} else {
		return nil, nil, fmt.Errorf("cannot found loadbalance id or name")
	}

	return conf, lb, err
}

// EnsureLoadBalancer creates a new load balancer 'name', or updates the existing one. Returns the status of the balancer
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (qc *QingCloud) EnsureLoadBalancer(ctx context.Context, _ string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	conf, lb, err := qc.getLoadBalancer(service)
	klog.V(4).Infof("==== EnsureLoadBalancer %s config %s ====", spew.Sdump(lb), spew.Sdump(conf))
	if err != nil && !errors.IsResourceNotFound(err) {
		klog.Errorf("getLoadBalancer for service %s error: %v", service.Name, err)
		return nil, err
	}

	// filter nodes by service externalTrafficPolicy
	nodes, e := qc.filterNodes(ctx, service, nodes, conf)
	if e != nil {
		klog.Errorf("filterNodes for service %s/%s with externalTrafficPolicy %s error: %v", service.Namespace, service.Name, service.Spec.ExternalTrafficPolicy, e)
		return nil, e
	}

	//1. ensure & update lb
	if err == nil {
		//The configuration of the load balancer will be independent, so we'll just create
		//it for now, not update it.

		//need modify attribute
		modify := false
		if result := needUpdateAttr(conf, lb); result != nil {
			if err = qc.Client.ModifyLB(result); err != nil {
				return nil, err
			}
			modify = true
		}

		// update eips
		err = qc.updateLBEip(conf, lb)
		if err != nil {
			klog.Errorf("update eip for lb %s error %v", *lb.Status.LoadBalancerID, err)
			return nil, err
		}

		//update listener
		listenerIDs := filterListeners(lb.Status.LoadBalancerListeners, conf.listenerName)
		klog.Infof("The loadbalancer %s has the following listeners %s", *lb.Status.LoadBalancerID, spew.Sdump(listenerIDs))
		if len(listenerIDs) <= 0 {
			klog.Infof("creating listeners for loadbalancers %s, service ports %s", *lb.Status.LoadBalancerID, spew.Sdump(service.Spec.Ports))
			if err = qc.createListenersAndBackends(conf, lb, service.Spec.Ports, nodes); err != nil {
				klog.Errorf("createListenersAndBackends for loadbalancer %s error: %v", *lb.Status.LoadBalancerID, err)
				return nil, err
			}
			modify = true
		} else {
			listeners, err := qc.Client.GetListeners(listenerIDs)
			if err != nil {
				return nil, err
			}

			//update listerner
			toDelete, toAdd := diffListeners(listeners, conf, service.Spec.Ports)
			if len(toDelete) > 0 {
				klog.Infof("listeners %s will be deleted for lb %s", spew.Sdump(toDelete), *lb.Status.LoadBalancerID)
				err = qc.Client.DeleteListener(toDelete)
				if err != nil {
					return nil, err
				}
				modify = true
			}

			if len(toAdd) > 0 {
				klog.Infof("listeners  %s will be added for lb %s", spew.Sdump(toAdd), *lb.Status.LoadBalancerID)
				err = qc.createListenersAndBackends(conf, lb, toAdd, nodes)
				if err != nil {
					return nil, err
				}
				modify = true
			}

			//update backend; for example, service annotation for backend label changed
			if len(toAdd) == 0 && len(toDelete) == 0 {
				for _, listener := range listeners {
					toDelete, toAdd := diffBackend(listener, nodes)

					if len(toDelete) > 0 {
						klog.Infof("backends %s will be deleted for listener %s(%s) of lb %s",
							spew.Sdump(toDelete), *listener.Spec.LoadBalancerListenerName, *listener.Spec.LoadBalancerListenerID, *lb.Status.LoadBalancerID)
						err = qc.Client.DeleteBackends(toDelete)
						if err != nil {
							return nil, err
						}
						modify = true
					}

					toAddBackends := generateLoadBalancerBackends(toAdd, listener, service.Spec.Ports)
					if len(toAddBackends) > 0 {
						klog.Infof("backends %s will be added for listener %s(%s) of lb %s",
							spew.Sdump(toAddBackends), *listener.Spec.LoadBalancerListenerName, *listener.Spec.LoadBalancerListenerID, *lb.Status.LoadBalancerID)
						_, err = qc.Client.CreateBackends(toAddBackends)
						if err != nil {
							return nil, err
						}
						modify = true
					}
				}
			}

			if !modify {
				klog.Infof("no change for listeners %v of lb %s, skip UpdateLB", spew.Sdump(listenerIDs), *lb.Status.LoadBalancerID)
				return convertLoadBalancerStatus(&lb.Status), nil
			}
		}

	} else if errors.IsResourceNotFound(err) {
		if conf.Policy == ReuseExistingLB || conf.Policy == Shared {
			return nil, err
		} else {
			err = nil
		}

		//1. create lb
		//1.1 prepare eip
		if len(conf.EipIDs) <= 0 && conf.EipSource != nil {
			eip, err := qc.prepareEip(conf.EipSource)
			if err != nil {
				return nil, err
			}
			conf.EipIDs = []*string{eip.Status.EIPID}
		}
		//1.2 prepare sg
		//default sg set by Client auto
		//1.3 create lb
		lb, err = qc.Client.CreateLB(&apis.LoadBalancer{
			Spec: apis.LoadBalancerSpec{
				LoadBalancerName: &conf.LoadBalancerName,
				LoadBalancerType: conf.LoadBalancerType,
				NodeCount:        conf.NodeCount,
				VxNetID:          conf.VxNetID,
				PrivateIPs:       []*string{conf.InternalIP},
				EIPs:             conf.EipIDs,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create loadbalance err=%+v", err)
		}

		//create listener
		if err = qc.createListenersAndBackends(conf, lb, service.Spec.Ports, nodes); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	if len(lb.Status.VIP) <= 0 {
		return nil, fmt.Errorf("loadbalance has no vip, please spec it")
	}

	err = qc.Client.UpdateLB(lb.Status.LoadBalancerID)
	if err != nil {
		return nil, err
	}

	return convertLoadBalancerStatus(&lb.Status), nil
}

// UpdateLoadBalancer updates hosts under the specified load balancer.
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (qc *QingCloud) UpdateLoadBalancer(ctx context.Context, _ string, service *v1.Service, nodes []*v1.Node) error {
	var modify bool

	conf, lb, err := qc.getLoadBalancer(service)
	klog.V(4).Infof("==== UpdateLoadBalancer %s config %s ====", spew.Sdump(lb), spew.Sdump(conf))
	if err != nil {
		klog.Errorf("getLoadBalancer for service %s error: %v", service.Name, err)
		return err
	}

	nodes, err = qc.filterNodes(ctx, service, nodes, conf)
	if err != nil {
		klog.Errorf("filterNodes for service %s with externalTrafficPolicy %s error: %v", service.Name, service.Spec.ExternalTrafficPolicy, err)
		return err
	}

	//update backend
	listenerIDs := filterListeners(lb.Status.LoadBalancerListeners, conf.listenerName)
	if len(listenerIDs) <= 0 {
		return nil
	}

	listeners, err := qc.Client.GetListeners(listenerIDs)
	if err != nil {
		return err
	}

	for _, listener := range listeners {
		toDelete, toAdd := diffBackend(listener, nodes)

		if len(toDelete) > 0 {
			klog.Infof("backends %s will be deleted for listener %s(%s) of lb %s",
				spew.Sdump(toDelete), *listener.Spec.LoadBalancerListenerName, *listener.Spec.LoadBalancerListenerID, *lb.Status.LoadBalancerID)
			err = qc.Client.DeleteBackends(toDelete)
			if err != nil {
				return err
			}
			modify = true
		}

		toAddBackends := generateLoadBalancerBackends(toAdd, listener, service.Spec.Ports)
		if len(toAddBackends) > 0 {
			klog.Infof("backends %s will be added for listener %s(%s) of lb %s",
				spew.Sdump(toAddBackends), *listener.Spec.LoadBalancerListenerName, *listener.Spec.LoadBalancerListenerID, *lb.Status.LoadBalancerID)
			_, err = qc.Client.CreateBackends(toAddBackends)
			if err != nil {
				return err
			}
			modify = true
		}
	}

	if !modify {
		klog.Infof("no backend change for listeners %v of lb %s, skip UpdateLB", spew.Sdump(listenerIDs), *lb.Status.LoadBalancerID)
		return nil
	}

	return qc.Client.UpdateLB(lb.Status.LoadBalancerID)
}

func (qc *QingCloud) createListenersAndBackends(conf *LoadBalancerConfig, status *apis.LoadBalancer, ports []v1.ServicePort, nodes []*v1.Node) error {
	listeners, err := generateLoadBalancerListeners(conf, status, ports)
	if err != nil {
		klog.Errorf("generateLoadBalancerListeners for loadbalancer %s error: %v", *status.Status.LoadBalancerID, err)
		return err
	}
	listeners, err = qc.Client.CreateListener(listeners)
	if err != nil {
		klog.Errorf("CreateListener for loadbalancer %s error: %v", *status.Status.LoadBalancerID, err)
		return err
	}

	//create backend
	for _, listener := range listeners {
		backends := generateLoadBalancerBackends(nodes, listener, ports)
		_, err = qc.Client.CreateBackends(backends)
		if err != nil {
			klog.Errorf("CreateBackends for loadbalancer %s error: %v", *status.Status.LoadBalancerID, err)
			return err
		}

	}

	return nil
}

// EnsureLoadBalancerDeleted deletes the specified load balancer if it
// exists, returning nil if the load balancer specified either didn't exist or
// was successfully deleted.
// This construction is useful because many cloud providers' load balancers
// have multiple underlying components, meaning a Get could say that the LB
// doesn't exist even if some part of it is still laying around.
// Implementations must treat the *v1.Service parameter as read-only and not modify it.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (qc *QingCloud) EnsureLoadBalancerDeleted(ctx context.Context, _ string, service *v1.Service) error {
	lbConfig, lb, err := qc.getLoadBalancer(service)
	klog.V(4).Infof("==== EnsureLoadBalancerDeleted %s config %s ====", spew.Sdump(lb), spew.Sdump(lbConfig))
	if errors.IsResourceNotFound(err) {
		return nil
	}

	if err != nil {
		return err
	}

	if lbConfig.ReuseLBID != "" {
		listeners := filterListeners(lb.Status.LoadBalancerListeners, lbConfig.listenerName)
		if len(listeners) <= 0 {
			return nil
		}
		err = qc.Client.DeleteListener(listeners)
		if err != nil {
			return err
		}
		return qc.Client.UpdateLB(lb.Status.LoadBalancerID)
	}

	//delete lb
	err = qc.Client.DeleteLB(lb.Status.LoadBalancerID)
	if err != nil {
		return err
	}

	//delete eip
	if len(lb.Status.CreatedEIPs) > 0 {
		err = qc.Client.ReleaseEIP(lb.Status.CreatedEIPs)
		if err != nil {
			return err
		}
	}

	//delete sg

	return nil
}

// filterNodes filter nodes by service externalTrafficPolicy and service backend label annotation
func (qc *QingCloud) filterNodes(ctx context.Context, svc *v1.Service, nodes []*v1.Node, lbconfog *LoadBalancerConfig) (newNodes []*v1.Node, err error) {

	if svc.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeLocal {
		klog.Infof("filter nodes for service %s/%s by real worker nodes", svc.Namespace, svc.Name)

		// 1. get endpoint to get real worker node
		nodeExists := make(map[string]bool)
		ep, err := qc.endpointInformer.Lister().Endpoints(svc.Namespace).Get(svc.Name)
		if err != nil {
			return nil, fmt.Errorf("get endpoint %s/%s error: %v", svc.Namespace, svc.Name, err)
		}
		for _, sub := range ep.Subsets {
			for _, addr := range sub.Addresses {
				nodeExists[*addr.NodeName] = true
			}
		}

		// 2. filter node
		for _, node := range nodes {
			if nodeExists[node.Name] {
				newNodes = append(newNodes, node)
			}
		}
	} else {
		if lbconfog.BackendLabel != "" {
			klog.Infof("filter nodes for service %s/%s by backend label: %s", svc.Namespace, svc.Name, lbconfog.BackendLabel)

			// filter by label
			labelMap, err := parseBackendLabel(lbconfog)
			if err != nil {
				return nil, fmt.Errorf("parseBackendLabel error: %v", err)
			}

			for i, node := range nodes {
				count := 0
				for k, v := range labelMap {
					l, ok := node.Labels[k]
					if ok && (l == v) {
						count++
					} else {
						break
					}
				}
				if count == len(labelMap) {
					newNodes = append(newNodes, nodes[i])
				}
			}
			// if there are no available nodes , use all nodes
			if len(newNodes) == 0 {
				klog.Infof("there are no available nodes for service %s/%s, use all nodes!", svc.Namespace, svc.Name)
				newNodes = nodes
			}
		} else {
			// no need to filter
			newNodes = nodes
		}
	}

	var resultNames []string
	for _, node := range newNodes {
		resultNames = append(resultNames, node.Name)
	}
	klog.Infof("filter node result for service %s/%s: %v", svc.Namespace, svc.Name, resultNames)

	return newNodes, nil
}
