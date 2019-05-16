package qcapiwrapper

import (
	"fmt"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/eip"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	"github.com/yunify/qingcloud-sdk-go/config"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog"
)

type QingcloudAPIWrapper struct {
	EipHelper       eip.EIPHelper
	LbExec          executor.QingCloudLoadBalancerExecutor
	SgExec          executor.QingCloudSecurityGroupExecutor
	InstanceService *qcservice.InstanceService
	UserID          string
}

func NewQingcloudAPIWrapper(qcConfig *config.Config, zone string) (*QingcloudAPIWrapper, error) {
	qcService, err := qcservice.Init(qcConfig)
	if err != nil {
		return nil, err
	}
	instanceService, err := qcService.Instance(zone)
	if err != nil {
		return nil, err
	}
	eipService, err := qcService.EIP(zone)
	if err != nil {
		return nil, err
	}
	lbService, err := qcService.LoadBalancer(zone)
	if err != nil {
		return nil, err
	}
	jobService, err := qcService.Job(zone)
	if err != nil {
		return nil, err
	}
	securityGroupService, err := qcService.SecurityGroup(zone)
	if err != nil {
		return nil, err
	}
	api, _ := qcService.Accesskey(zone)
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
	userid := *output.AccessKeySet[0].Owner
	apiwrapper := &QingcloudAPIWrapper{
		EipHelper: eip.NewEIPHelperOfQingCloud(eip.NewEIPHelperOfQingCloudOption{
			JobAPI: jobService,
			EIPAPI: eipService,
			UserID: userid,
		}),
		LbExec:          executor.NewQingCloudLoadBalanceExecutor(lbService, jobService, userid),
		SgExec:          executor.NewQingCloudSecurityGroupExecutor(securityGroupService, userid),
		InstanceService: instanceService,
		UserID:          userid,
	}
	return apiwrapper, nil
}
