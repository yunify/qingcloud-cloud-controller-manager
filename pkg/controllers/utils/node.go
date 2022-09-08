package utils

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

// check if the node should be backend or not
func NodeConditionCheck(node *corev1.Node) bool {
	if _, hasExcludeBalancerLabel := node.Labels[corev1.LabelNodeExcludeBalancers]; hasExcludeBalancerLabel {
		return false
	}

	// If we have no info, don't accept
	if len(node.Status.Conditions) == 0 {
		return false
	}
	for _, cond := range node.Status.Conditions {
		// We consider the node for load balancing only when its NodeReady condition status is ConditionTrue
		if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
			klog.V(4).Infof("Ignoring node %v with %v condition status %v", node.Name, cond.Type, cond.Status)
			return false
		}
	}
	return true
}
