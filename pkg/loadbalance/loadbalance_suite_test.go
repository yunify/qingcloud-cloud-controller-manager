package loadbalance_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestLoadbalance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Loadbalance Suite")
}
