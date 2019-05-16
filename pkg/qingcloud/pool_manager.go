package qingcloud

import (
	"fmt"
	"strconv"
	"time"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/eip"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/pool"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/qcapiwrapper"
	"k8s.io/klog"
)

const (
	PoolReconcileInterval   = time.Second * 10
	SharedSecurityGroupName = "DONT_MODIFY_THIS_SHARED_SECURITY_GROUP_BY_LB"
)

var (
	ErrorEIPNotSuffcient = fmt.Errorf("EIP Pool is EMPTY")
)

type poolManager struct {
	lbExec           executor.QingCloudLoadBalancerExecutor
	sgExec           executor.QingCloudSecurityGroupExecutor
	eipHelper        eip.EIPHelper
	pool             *pool.LBPool
	eipPool          *pool.EIPPool
	useAvailableEIPs bool

	maxLBInPool   int
	minLBInPool   int
	sgID          string
	defaultLBType int
}

func newPoolManager(qcapi *qcapiwrapper.QingcloudAPIWrapper, useAvailableEIPs bool, eips ...string) (*poolManager, error) {
	pm := &poolManager{
		maxLBInPool:      2,
		minLBInPool:      1,
		defaultLBType:    0,
		useAvailableEIPs: useAvailableEIPs,
		pool:             pool.NewLBPool(),
		eipPool:          pool.NewEIPPool(),
		lbExec:           qcapi.LbExec,
		sgExec:           qcapi.SgExec,
		eipHelper:        qcapi.EipHelper,
	}
	klog.V(2).Infoln("Initialize lb pools")
	err := pm.initPool(eips...)
	return pm, err
}

func (p *poolManager) GetPool() *pool.LBPool {
	return p.pool
}
func (p *poolManager) initPool(eips ...string) error {
	lbs, err := p.lbExec.ListByPrefix(pool.PoolLBPrefixName)
	if err != nil {
		return err
	}
	for _, lb := range lbs {
		if len(lb.Cluster) < 1 {
			klog.V(2).Infof("Skip adding lb %s because it does not have an eip", *lb.LoadBalancerID)
			continue
		}
		err = p.pool.AddLBToPool(pool.QclbToPoolElement(lb))
		if err != nil {
			klog.Errorf("Failed to add lb %s to pool", *lb.LoadBalancerID)
			return err
		}
	}
	sg, err := p.sgExec.GetSecurityGroupByName(SharedSecurityGroupName)
	if err != nil {
		if executor.IsQcResourceNotFound(err) {
			klog.V(2).Infoln("Creating shared sg for lb pool")
			sg, err := p.sgExec.CreateSecurityGroup(SharedSecurityGroupName, executor.DefaultLBSecurityGroupRules)
			if err != nil {
				klog.Errorln("Failed to create shared sg in cloud")
				return err
			}
			p.sgID = *sg.SecurityGroupID
			return nil
		}
	} else {
		p.sgID = *sg.SecurityGroupID
	}
	for _, ipid := range eips {
		e, err := p.eipHelper.GetEIPByID(ipid)
		if err != nil {
			if err == eip.ErrorEIPNotFound {
				klog.Warningf("Cannot find eip %s in qingcloud, skipping", ipid)
				continue
			}
			klog.Errorf("Failed to get eip %s in cloud", ipid)
			return err
		}
		p.eipPool.AddEIP(e)
	}
	return nil
}

func (p *poolManager) StartLBPoolManager(stop <-chan struct{}) {
	klog.V(1).Infoln("Starting LB pooling")
	for {
		select {
		case <-stop:
			klog.V(2).Infoln("Begin to clean pool")
			return
		default:
			p.updateEIPPool()
			p.updatePool()
			time.Sleep(PoolReconcileInterval)
		}
	}
}
func (p *poolManager) updateEIPPool() {
	klog.V(2).Infof("Current eippool length [%d]", p.eipPool.Length())
	if p.eipPool.Length() == 0 {
		if !p.useAvailableEIPs {
			eip, err := p.eipHelper.AllocateEIP()
			if err != nil {
				klog.Warningf("Could not allocate any eips, err: %s", err.Error())
				return
			}
			p.eipPool.AddEIP(eip)
		} else {
			eip, err := p.eipHelper.GetAvaliableOrAllocateEIP()
			if err != nil {
				klog.Warningf("Could not get or allocate any eips, err: %s", err.Error())
				return
			}
			p.eipPool.AddEIP(eip)
		}
	} else if p.eipPool.Length() > 1 {
		eip := p.eipPool.PopReleasableEIP()
		if eip != nil {
			klog.V(3).Infof("Release unused eip %s", eip.ID)
			err := p.eipHelper.ReleaseEIP(eip.ID)
			if err != nil {
				klog.Errorf("Failed to release eip %s, return to pool", eip.ID)
				p.eipPool.AddEIP(eip)
			}
		}
	}
}

func (p *poolManager) updatePool() {
	klog.V(2).Infoln("Begin to updating lb pool")
	klog.V(2).Infof("Current pool length [%d]", p.pool.Length())
	if p.pool.Length() < p.minLBInPool {
		err := p.increaseLB()
		if err != nil {
			klog.Errorf("Failed to increase lb in pool,err %s", err.Error())
		}
	}
	if p.pool.Length() > p.maxLBInPool {
		err := p.decreaseLB()
		if err != nil {
			klog.Errorf("Failed to decrease lb in pool,err %s", err.Error())
		}
	}
}

func (p *poolManager) increaseLB() error {
	name := pool.PoolLBPrefixName + strconv.Itoa(p.pool.Length())
	klog.V(2).Infof("Add lb %s to pool", name)
	eip := p.eipPool.PopEIP()
	if eip == nil {
		return ErrorEIPNotSuffcient
	}
	lb, err := p.lbExec.Create(name, p.sgID, p.defaultLBType, eip.ID)
	if err != nil {
		klog.Errorln("Falied to create lb in pool")
		p.eipPool.AddEIP(eip)
		return err
	}
	return p.pool.AddLBToPool(pool.QclbToPoolElement(lb))
}

func (p *poolManager) decreaseLB() error {
	lb := p.pool.PopLB()
	klog.V(2).Infof("Deleting lb %s in pool", lb.ID)
	err := p.lbExec.Delete(lb.ID)
	if err != nil {
		p.pool.AddLBToPool(lb)
		return err
	}
	if lb.EIP.Source == eip.AllocateOnly {
		p.eipHelper.ReleaseEIP(lb.EIP.ID)
	} else {
		p.eipPool.AddEIP(lb.EIP)
	}
	return nil
}
