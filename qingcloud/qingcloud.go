// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package qingcloud

// Please see qingcloud document: https://docs.qingcloud.com/index.html
// and must pay attention to your account resource quota limit.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	qcconfig "github.com/yunify/qingcloud-sdk-go/config"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	gcfg "gopkg.in/gcfg.v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
)

const (
	ProviderName = "qingcloud"
)

type Config struct {
	Global struct {
		QYConfigPath      string `gcfg:"qyConfigPath"`
		Zone              string `gcfg:"zone"`
		DefaultVxNetForLB string `gcfg:"defaultVxNetForLB"`
		ClusterID         string `gcfg:"clusterID"`
	}
}

// A single Kubernetes cluster can run in multiple zones,
// but only within the same region (and cloud provider).
type QingCloud struct {
	instanceService      *qcservice.InstanceService
	lbService            *qcservice.LoadBalancerService
	volumeService        *qcservice.VolumeService
	jobService           *qcservice.JobService
	securityGroupService *qcservice.SecurityGroupService
	zone                 string
	selfInstance         *qcservice.Instance
	defaultVxNetForLB    string
	clusterID            string

	k8sclient *kubernetes.Clientset
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		cfg, err := readConfig(config)
		if err != nil {
			return nil, err
		}
		return newQingCloud(cfg)
	})
}

func readConfig(config io.Reader) (Config, error) {
	if config == nil {
		err := fmt.Errorf("no qingcloud provider config file given")
		return Config{}, err
	}

	var cfg Config
	err := gcfg.ReadInto(&cfg, config)
	return cfg, err
}

// newQingCloud returns a new instance of QingCloud cloud provider.
func newQingCloud(config Config) (cloudprovider.Interface, error) {
	qcConfig, err := qcconfig.NewDefault()
	if err != nil {
		return nil, err
	}
	if err = qcConfig.LoadConfigFromFilepath(config.Global.QYConfigPath); err != nil {
		return nil, err
	}

	qcService, err := qcservice.Init(qcConfig)
	if err != nil {
		return nil, err
	}
	instanceService, err := qcService.Instance(config.Global.Zone)
	if err != nil {
		return nil, err
	}
	lbService, err := qcService.LoadBalancer(config.Global.Zone)
	if err != nil {
		return nil, err
	}
	volumeService, err := qcService.Volume(config.Global.Zone)
	if err != nil {
		return nil, err
	}
	jobService, err := qcService.Job(config.Global.Zone)
	if err != nil {
		return nil, err
	}
	securityGroupService, err := qcService.SecurityGroup(config.Global.Zone)
	if err != nil {
		return nil, err
	}

	//init k8sclientset
	k8sconfig, err := rest.InClusterConfig()
	if err != nil {
		klog.Errorln("Failed to load in cluster config")
		return nil, err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(k8sconfig)
	if err != nil {
		klog.Errorln("Failed to generate k8s clientset")
		return nil, err
	}
	qc := QingCloud{
		instanceService:      instanceService,
		lbService:            lbService,
		volumeService:        volumeService,
		jobService:           jobService,
		securityGroupService: securityGroupService,
		zone:                 config.Global.Zone,
		defaultVxNetForLB:    config.Global.DefaultVxNetForLB,
		clusterID:            config.Global.ClusterID,
		k8sclient:            clientset,
	}
	host, err := getHostname()
	if err != nil {
		return nil, err
	}
	ins, err := qc.GetInstanceByID(context.TODO(), host)
	if err != nil {
		klog.Errorf("Get self instance fail, id: %s, err: %s", host, err.Error())
		return nil, err
	}
	qc.selfInstance = ins

	klog.V(1).Infof("QingCloud provider init finish, zone: %v, selfInstance: %+v", qc.zone, qc.selfInstance)

	return &qc, nil
}

func (qc *QingCloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {

}

// LoadBalancer returns an implementation of LoadBalancer for QingCloud.
func (qc *QingCloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	klog.V(4).Info("LoadBalancer() called")
	return qc, true
}

// Instances returns an implementation of Instances for QingCloud.
func (qc *QingCloud) Instances() (cloudprovider.Instances, bool) {
	klog.V(4).Info("Instances() called")
	return qc, true
}

func (qc *QingCloud) Zones() (cloudprovider.Zones, bool) {
	return qc, true
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

// ScrubDNS filters DNS settings for pods.
func (qc *QingCloud) ScrubDNS(nameservers, searches []string) (nsOut, srchOut []string) {
	return nameservers, searches
}

func (qc *QingCloud) GetZone(ctx context.Context) (cloudprovider.Zone, error) {
	klog.V(4).Infof("GetZone() called, current zone is %v", qc.zone)

	return cloudprovider.Zone{Region: qc.zone}, nil
}

func getHostname() (string, error) {
	content, err := ioutil.ReadFile("/etc/qingcloud/instance-id")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// HasClusterID returns true if the cluster has a clusterID
func (qc *QingCloud) HasClusterID() bool {
	return qc.clusterID != ""
}

// GetZoneByNodeName implements Zones.GetZoneByNodeName
// This is particularly useful in external cloud providers where the kubelet
// does not initialize node data.
func (qc *QingCloud) GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error) {
	klog.V(4).Infof("GetZoneByNodeName() called, current zone is %v, and return zone directly as temporary solution", qc.zone)
	return cloudprovider.Zone{Region: qc.zone}, nil
}

// GetZoneByProviderID implements Zones.GetZoneByProviderID
// This is particularly useful in external cloud providers where the kubelet
// does not initialize node data.
func (qc *QingCloud) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	klog.V(4).Infof("GetZoneByProviderID() called, current zone is %v, and return zone directly as temporary solution", qc.zone)
	return cloudprovider.Zone{Region: qc.zone}, nil
}

// InstanceExistsByProviderID returns true if the instance with the given provider id still exists and is running.
// If false is returned with no error, the instance will be immediately deleted by the cloud controller manager.
func (qc *QingCloud) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	return false, errors.New("unimplemented")
}
