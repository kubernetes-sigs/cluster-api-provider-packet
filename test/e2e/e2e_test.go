package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
)

var _ = Describe("functional tests", func() {
	var ()

	Describe("workload cluster lifecycle", func() {
		It("It should be creatable and deletable", func() {
			time.Sleep(5 * time.Second)
			By("Create cluster")
		})
	})
})
