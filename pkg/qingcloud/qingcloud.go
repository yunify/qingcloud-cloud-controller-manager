// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package qingcloud

// Please see qingcloud document: https://docs.qingcloud.com/index.html
// and must pay attention to your account resource quota limit.

import (
	"fmt"
	"io"

	qcconfig "github.com/yunify/qingcloud-sdk-go/config"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	gcfg "gopkg.in/gcfg.v1"
	"k8s.io/client-go/informers"
	corev1informer "k8s.io/client-go/informers/core/v1"
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
		IsApp             bool   `gcfg:"isApp"`
	}
}

var _ cloudprovider.Interface = &QingCloud{}

// A single Kubernetes cluster can run in multiple zones,
// but only within the same region (and cloud provider).
type QingCloud struct {
	instanceService      *qcservice.InstanceService
	lbService            *qcservice.LoadBalancerService
	volumeService        *qcservice.VolumeService
	jobService           *qcservice.JobService
	eipService           *qcservice.EIPService
	securityGroupService *qcservice.SecurityGroupService

	isAPP             bool
	zone              string
	defaultVxNetForLB string
	clusterID         string
	userID            string

	nodeInformer    corev1informer.NodeInformer
	serviceInformer corev1informer.ServiceInformer
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
	eipService, _ := qcService.EIP(config.Global.Zone)
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
	api, _ := qcService.Accesskey(config.Global.Zone)
	output, err := api.DescribeAccessKeys(&qcservice.DescribeAccessKeysInput{
		AccessKeys: []*string{&qcConfig.AccessKeyID},
	})
	if err != nil {
		klog.Errorf("Failed to get userID")
		return nil, err
	}
	if len(output.AccessKeySet) == 0 {
		err = fmt.Errorf("AccessKey %s have not userid", qcConfig.AccessKeyID)
		return nil, err
	}
	qc := QingCloud{
		instanceService:      instanceService,
		lbService:            lbService,
		volumeService:        volumeService,
		jobService:           jobService,
		securityGroupService: securityGroupService,
		eipService:           eipService,
		zone:                 config.Global.Zone,
		defaultVxNetForLB:    config.Global.DefaultVxNetForLB,
		clusterID:            config.Global.ClusterID,
		userID:               *output.AccessKeySet[0].Owner,
	}

	klog.V(1).Infof("QingCloud provider init done")
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

// HasClusterID returns true if the cluster has a clusterID
func (qc *QingCloud) HasClusterID() bool {
	return qc.clusterID != ""
}
