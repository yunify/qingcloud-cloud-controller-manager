// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package qingcloud

// Please see qingcloud document: https://docs.qingcloud.com/index.html
// and must pay attention to your account resource quota limit.

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	yaml "gopkg.in/yaml.v2"
	"io"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	corev1informer "k8s.io/client-go/informers/core/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
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

	nodeInformer    corev1informer.NodeInformer
	serviceInformer corev1informer.ServiceInformer
	corev1interface corev1.CoreV1Interface
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

	conf, err = ParseServiceLBConfig(qc.Config.ClusterID, service)
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

	//1. ensure & update lb
	if err == nil {
		//The configuration of the load balancer will be independent, so we'll just create
		//it for now, not update it.

		//need modify attribute
		if result := needUpdateAttr(conf, lb); result != nil {
			if err = qc.Client.ModifyLB(result); err != nil {
				return nil, err
			}
		}

		//update listener
		listenerIDs := filterListeners(lb.Status.LoadBalancerListeners, conf.listenerName)
		klog.Infof("The loadbalancer %s has the following listeners %s", *lb.Status.LoadBalancerID, spew.Sdump(listenerIDs))
		if len(listenerIDs) <= 0 {
			klog.Infof("create listeners for loadbalancers %s, service ports %s", *lb.Status.LoadBalancerID, spew.Sdump(service.Spec.Ports))
			if err = qc.createListenersAndBackends(conf, lb, service.Spec.Ports, nodes); err != nil {
				return nil, err
			}
		} else {
			listeners, err := qc.Client.GetListeners(listenerIDs)
			if err != nil {
				return nil, err
			}

			toDelete, toAdd := diffListeners(listeners, service.Spec.Ports)
			klog.Infof("listeners %s will be deleted, %s will be added", spew.Sdump(toDelete), spew.Sdump(toAdd))

			if len(toDelete) > 0 {
				err = qc.Client.DeleteListener(toDelete)
				if err != nil {
					return nil, err
				}
			}

			if len(toAdd) > 0 {
				err = qc.createListenersAndBackends(conf, lb, toAdd, nodes)
				if err != nil {
					return nil, err
				}
			}
		}

		//update backend

	} else if errors.IsResourceNotFound(err) {
		if conf.Policy == ReuseExistingLB || conf.Policy == Shared {
			return nil, err
		} else {
			err = nil
		}

		//1. create lb
		//1.1 prepare eip
		if len(conf.EipIDs) <= 0 && conf.EipSource != nil {
			var (
				eip *apis.EIP
			)

			switch *conf.EipSource {
			case AllocateOnly:
				eip, err = qc.Client.AllocateEIP(nil)
			case UseAvailableOnly:
				eips, err := qc.Client.GetAvaliableEIPs()
				if err != nil {
					return nil, err
				}

				if len(eips) <= 0 {
					return nil, fmt.Errorf("no avaliable eips")
				}

				eip = eips[0]
			case UseAvailableOrAllocateOne:
				eips, err := qc.Client.GetAvaliableEIPs()
				if err != nil {
					return nil, err
				}

				if len(eips) <= 0 {
					eip, err = qc.Client.AllocateEIP(nil)
					if err != nil {
						return nil, err
					}
				} else {
					eip = eips[0]
				}
			}

			if err != nil {
				return nil, err
			} else if eip == nil {
				return nil, fmt.Errorf("has no eip")
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
		return nil, fmt.Errorf("loadbalance has not vip, please spec it")
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
	conf, lb, err := qc.getLoadBalancer(service)
	if err != nil {
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

	var (
		toDeleteBackends []*string
		toAddBackends    []*apis.LoadBalancerBackend
	)
	for _, listener := range listeners {
		toDelete, toAdd := diffBackend(listener, nodes)
		toDeleteBackends = append(toDeleteBackends, toDelete...)
		toAddBackends = append(toAddBackends, generateLoadBalancerBackends(toAdd, listener, service.Spec.Ports)...)
	}
	if len(toDeleteBackends) > 0 {
		err = qc.Client.DeleteBackends(toDeleteBackends)
		if err != nil {
			return err
		}
	}
	if len(toAddBackends) > 0 {
		_, err = qc.Client.CreateBackends(toAddBackends)
		if err != nil {
			return err
		}
	}

	err = qc.Client.UpdateLB(lb.Status.LoadBalancerID)
	if err != nil {
		return err
	}

	return nil
}

func (qc *QingCloud) createListenersAndBackends(conf *LoadBalancerConfig, status *apis.LoadBalancer, ports []v1.ServicePort, nodes []*v1.Node) error {
	listeners, err := generateLoadBalancerListeners(conf, status, ports)
	if err != nil {
		return err
	}
	listeners, err = qc.Client.CreateListener(listeners)
	if err != nil {
		return err
	}

	//create backend
	for _, listener := range listeners {
		backends := generateLoadBalancerBackends(nodes, listener, ports)
		_, err = qc.Client.CreateBackends(backends)
		if err != nil {
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
