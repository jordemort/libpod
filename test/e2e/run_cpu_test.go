// +build !remoteclient

package integration

import (
	"os"

	. "github.com/containers/libpod/test/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Podman run cpu", func() {
	var (
		tempdir    string
		err        error
		podmanTest *PodmanTestIntegration
	)

	BeforeEach(func() {
		tempdir, err = CreateTempDirInTempDir()
		if err != nil {
			os.Exit(1)
		}
		podmanTest = PodmanTestCreate(tempdir)
		podmanTest.Setup()
		podmanTest.SeedImages()
	})

	AfterEach(func() {
		podmanTest.Cleanup()
		f := CurrentGinkgoTestDescription()
		processTestResult(f)

	})

	It("podman run cpu-period", func() {
		SkipIfRootless()
		result := podmanTest.Podman([]string{"run", "--rm", "--cpu-period=5000", ALPINE, "cat", "/sys/fs/cgroup/cpu/cpu.cfs_period_us"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))
		Expect(result.OutputToString()).To(Equal("5000"))
	})

	It("podman run cpu-quota", func() {
		SkipIfRootless()
		result := podmanTest.Podman([]string{"run", "--rm", "--cpu-quota=5000", ALPINE, "cat", "/sys/fs/cgroup/cpu/cpu.cfs_quota_us"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))
		Expect(result.OutputToString()).To(Equal("5000"))
	})

	It("podman run cpus", func() {
		SkipIfRootless()
		result := podmanTest.Podman([]string{"run", "--rm", "--cpus=0.5", ALPINE, "cat", "/sys/fs/cgroup/cpu/cpu.cfs_period_us"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))
		Expect(result.OutputToString()).To(Equal("100000"))

		result = podmanTest.Podman([]string{"run", "--rm", "--cpus=0.5", ALPINE, "cat", "/sys/fs/cgroup/cpu/cpu.cfs_quota_us"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))
		Expect(result.OutputToString()).To(Equal("50000"))
	})

	It("podman run cpu-shares", func() {
		SkipIfRootless()
		result := podmanTest.Podman([]string{"run", "--rm", "--cpu-shares=2", ALPINE, "cat", "/sys/fs/cgroup/cpu/cpu.shares"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))
		Expect(result.OutputToString()).To(Equal("2"))
	})

	It("podman run cpuset-cpus", func() {
		SkipIfRootless()
		result := podmanTest.Podman([]string{"run", "--rm", "--cpuset-cpus=0", ALPINE, "cat", "/sys/fs/cgroup/cpuset/cpuset.cpus"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))
		Expect(result.OutputToString()).To(Equal("0"))
	})

	It("podman run cpuset-mems", func() {
		SkipIfRootless()
		result := podmanTest.Podman([]string{"run", "--rm", "--cpuset-mems=0", ALPINE, "cat", "/sys/fs/cgroup/cpuset/cpuset.mems"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))
		Expect(result.OutputToString()).To(Equal("0"))
	})

	It("podman run cpus and cpu-period", func() {
		result := podmanTest.Podman([]string{"run", "--rm", "--cpu-period=5000", "--cpus=0.5", ALPINE, "ls"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Not(Equal(0)))
	})

	It("podman run cpus and cpu-quota", func() {
		result := podmanTest.Podman([]string{"run", "--rm", "--cpu-quota=5000", "--cpus=0.5", ALPINE, "ls"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Not(Equal(0)))
	})
})
