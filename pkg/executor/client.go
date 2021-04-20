package executor

import (
	"fmt"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
	qcconfig "github.com/yunify/qingcloud-sdk-go/config"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"time"
)

const (
	ResourceNameLoadBalancer  = "LoadBalancer"
	ResourceNameListener      = "Listener"
	ResourceNameBackend       = "Backend"
	ResourceNameSecurityGroup = "SecurityGroup"

	SGTagResourceType  = "security_group"
	EIPTagResourceType = "eip"

	waitInterval         = 5 * time.Second
	eipWaitInterval      = 2 * time.Second
	sgWaitInterval       = 2 * time.Second
	operationWaitTimeout = 180 * time.Second
	pageLimt             = 100
)

// QingCloud is the main entry of all interface
type QingCloudClient struct {
	InstanceService      *qcservice.InstanceService
	LBService            *qcservice.LoadBalancerService
	jobService           *qcservice.JobService
	EIPService           *qcservice.EIPService
	securityGroupService *qcservice.SecurityGroupService
	tagService           *qcservice.TagService

	Config *ClientConfig
	sg     *apis.SecurityGroup
}

var _ QingCloudClientInterface = &QingCloudClient{}

type QingCloudClientInterface interface {
	//LoadBalancer
	GetLoadBalancerByName(name string) (*apis.LoadBalancer, error)
	GetLoadBalancerByID(id string) (*apis.LoadBalancer, error)
	ModifyLB(conf *apis.LoadBalancer) error
	CreateLB(input *apis.LoadBalancer) (*apis.LoadBalancer, error)
	DeleteLB(id *string) error
	UpdateLB(id *string) error

	//sg
	GetSecurityGroupByName(name string) (*apis.SecurityGroup, error)

	//EIP
	AllocateEIP(eip *apis.EIP) (*apis.EIP, error)
	GetAvaliableEIPs() ([]*apis.EIP, error)
	ReleaseEIP(ids []*string) error

	//backend
	CreateBackends(backends []*apis.LoadBalancerBackend) ([]*string, error)
	DeleteBackends(ids []*string) error

	//listener
	CreateListener(inputs []*apis.LoadBalancerListener) ([]*apis.LoadBalancerListener, error)
	GetListeners(id []*string) ([]*apis.LoadBalancerListener, error)
	DeleteListener(lsnid []*string) error
}

type ClientConfig struct {
	IsAPP  bool
	UserID string
	TagIDs []string
}

func NewQingCloudClient(config *ClientConfig, path string) (QingCloudClientInterface, error) {
	qcConfig, err := qcconfig.NewDefault()
	if err != nil {
		return nil, err
	}

	if err = qcConfig.LoadConfigFromFilepath(path); err != nil {
		return nil, err
	}

	qcService, err := qcservice.Init(qcConfig)
	if err != nil {
		return nil, err
	}

	instanceService, err := qcService.Instance(qcConfig.Zone)
	if err != nil {
		return nil, err
	}

	eipService, _ := qcService.EIP(qcConfig.Zone)

	lbService, err := qcService.LoadBalancer(qcConfig.Zone)
	if err != nil {
		return nil, err
	}

	jobService, err := qcService.Job(qcConfig.Zone)
	if err != nil {
		return nil, err
	}

	securityGroupService, err := qcService.SecurityGroup(qcConfig.Zone)
	if err != nil {
		return nil, err
	}

	tagService, _ := qcService.Tag(qcConfig.Zone)

	api, _ := qcService.Accesskey(qcConfig.Zone)
	output, err := api.DescribeAccessKeys(&qcservice.DescribeAccessKeysInput{
		AccessKeys: []*string{&qcConfig.AccessKeyID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get user id, err=%v", err)
	}

	config.UserID = *output.AccessKeySet[0].Owner
	qc := QingCloudClient{
		InstanceService:      instanceService,
		LBService:            lbService,
		tagService:           tagService,
		jobService:           jobService,
		securityGroupService: securityGroupService,
		EIPService:           eipService,
		Config:               config,
	}

	sg, err := qc.ensureSecurityGroupByName(DefaultSgName)
	if err != nil {
		return nil, err
	}
	qc.sg = sg

	return &qc, nil
}
