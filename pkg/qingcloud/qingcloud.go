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
	yaml "gopkg.in/yaml.v2"
	"k8s.io/client-go/informers"
	corev1informer "k8s.io/client-go/informers/core/v1"
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
}

var _ cloudprovider.Interface = &QingCloud{}

// A single Kubernetes cluster can run in multiple zones,
// but only within the same region (and cloud provider).

// QingCloud is the main entry of all interface
type QingCloud struct {
	instanceService      *qcservice.InstanceService
	lbService            *qcservice.LoadBalancerService
	volumeService        *qcservice.VolumeService
	jobService           *qcservice.JobService
	eipService           *qcservice.EIPService
	securityGroupService *qcservice.SecurityGroupService
	tagService           *qcservice.TagService

	isAPP             bool
	zone              string
	defaultVxNetForLB string
	clusterID         string
	userID            string
	tagIDs            []string

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
		return Config{}, fmt.Errorf("no qingcloud provider config file given")
	}

	var cfg Config
	err := yaml.NewDecoder(config).Decode(&cfg)
	return cfg, err
}

// newQingCloud returns a new instance of QingCloud cloud provider.
func newQingCloud(config Config) (cloudprovider.Interface, error) {
	qcConfig, err := qcconfig.NewDefault()
	if err != nil {
		return nil, err
	}
	if err = qcConfig.LoadConfigFromFilepath(QYConfigPath); err != nil {
		return nil, err
	}
	qcService, err := qcservice.Init(qcConfig)
	if err != nil {
		return nil, err
	}
	instanceService, err := qcService.Instance(config.Zone)
	if err != nil {
		return nil, err
	}
	eipService, _ := qcService.EIP(config.Zone)
	lbService, err := qcService.LoadBalancer(config.Zone)
	if err != nil {
		return nil, err
	}
	volumeService, err := qcService.Volume(config.Zone)
	if err != nil {
		return nil, err
	}
	jobService, err := qcService.Job(config.Zone)
	if err != nil {
		return nil, err
	}
	securityGroupService, err := qcService.SecurityGroup(config.Zone)
	if err != nil {
		return nil, err
	}
	api, _ := qcService.Accesskey(config.Zone)
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
		zone:                 config.Zone,
		defaultVxNetForLB:    config.DefaultVxNetForLB,
		clusterID:            config.ClusterID,
		userID:               *output.AccessKeySet[0].Owner,
		isAPP:                config.IsApp,
	}
	if len(config.TagIDs) > 0 {
		klog.V(2).Infoln("Init tag service as tagid is not empty")
		tagService, _ := qcService.Tag(config.Zone)
		qc.tagService = tagService
		qc.tagIDs = config.TagIDs
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
