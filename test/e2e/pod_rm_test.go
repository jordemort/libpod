package integration

import (
	"fmt"
	"os"

	. "github.com/containers/libpod/test/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Podman pod rm", func() {
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
		podmanTest.RestoreAllArtifacts()
	})

	AfterEach(func() {
		podmanTest.CleanupPod()
		f := CurrentGinkgoTestDescription()
		processTestResult(f)

	})

	It("podman pod rm empty pod", func() {
		_, ec, podid := podmanTest.CreatePod("")
		Expect(ec).To(Equal(0))

		result := podmanTest.Podman([]string{"pod", "rm", podid})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))
	})

	It("podman pod rm latest pod", func() {
		_, ec, podid := podmanTest.CreatePod("")
		Expect(ec).To(Equal(0))

		_, ec2, podid2 := podmanTest.CreatePod("")
		Expect(ec2).To(Equal(0))

		result := podmanTest.Podman([]string{"pod", "rm", "--latest"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))

		result = podmanTest.Podman([]string{"pod", "ps", "-q", "--no-trunc"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))
		Expect(result.OutputToString()).To(ContainSubstring(podid))
		Expect(result.OutputToString()).To(Not(ContainSubstring(podid2)))
	})

	It("podman pod rm doesn't remove a pod with a container", func() {
		_, ec, podid := podmanTest.CreatePod("")
		Expect(ec).To(Equal(0))

		_, ec2, _ := podmanTest.RunLsContainerInPod("", podid)
		Expect(ec2).To(Equal(0))

		result := podmanTest.Podman([]string{"pod", "rm", podid})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(125))

		result = podmanTest.Podman([]string{"ps", "-qa"})
		result.WaitWithDefaultTimeout()
		Expect(len(result.OutputToStringArray())).To(Equal(1))
	})

	It("podman pod rm -f does remove a running container", func() {
		_, ec, podid := podmanTest.CreatePod("")
		Expect(ec).To(Equal(0))

		session := podmanTest.RunTopContainerInPod("", podid)
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).To(Equal(0))

		result := podmanTest.Podman([]string{"pod", "rm", "-f", podid})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))

		result = podmanTest.Podman([]string{"ps", "-q"})
		result.WaitWithDefaultTimeout()
		Expect(result.OutputToString()).To(BeEmpty())
	})

	It("podman pod rm -a doesn't remove a running container", func() {
		fmt.Printf("To start, there are %d pods\n", podmanTest.NumberOfPods())
		_, ec, podid1 := podmanTest.CreatePod("")
		Expect(ec).To(Equal(0))

		_, ec, _ = podmanTest.CreatePod("")
		Expect(ec).To(Equal(0))
		fmt.Printf("Started %d pods\n", podmanTest.NumberOfPods())

		session := podmanTest.RunTopContainerInPod("", podid1)
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).To(Equal(0))
		podmanTest.WaitForContainer()
		Expect(podmanTest.NumberOfContainersRunning()).To(Equal(1))
		fmt.Printf("Started container running in one pod")

		num_pods := podmanTest.NumberOfPods()
		Expect(num_pods).To(Equal(2))
		ps := podmanTest.Podman([]string{"pod", "ps"})
		ps.WaitWithDefaultTimeout()
		fmt.Printf("Current %d pod(s):\n%s\n", num_pods, ps.OutputToString())

		fmt.Printf("Removing all empty pods\n")
		result := podmanTest.Podman([]string{"pod", "rm", "-a"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Not(Equal(0)))
		foundExpectedError, _ := result.ErrorGrepString("contains containers and cannot be removed")
		Expect(foundExpectedError).To(Equal(true))

		num_pods = podmanTest.NumberOfPods()
		ps = podmanTest.Podman([]string{"pod", "ps"})
		ps.WaitWithDefaultTimeout()
		fmt.Printf("Final %d pod(s):\n%s\n", num_pods, ps.OutputToString())
		Expect(num_pods).To(Equal(1))
		// Confirm top container still running inside remaining pod
		Expect(podmanTest.NumberOfContainersRunning()).To(Equal(1))
	})

	It("podman pod rm -fa removes everything", func() {
		_, ec, podid1 := podmanTest.CreatePod("")
		Expect(ec).To(Equal(0))

		_, ec, podid2 := podmanTest.CreatePod("")
		Expect(ec).To(Equal(0))

		_, ec, _ = podmanTest.CreatePod("")
		Expect(ec).To(Equal(0))

		session := podmanTest.RunTopContainerInPod("", podid1)
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).To(Equal(0))

		session = podmanTest.Podman([]string{"create", "-d", "--pod", podid1, ALPINE, "ls"})
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).To(Equal(0))

		_, ec, _ = podmanTest.RunLsContainerInPod("", podid2)
		Expect(ec).To(Equal(0))

		session = podmanTest.RunTopContainerInPod("", podid2)
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).To(Equal(0))

		result := podmanTest.Podman([]string{"pod", "rm", "-fa"})
		result.WaitWithDefaultTimeout()
		Expect(result.ExitCode()).To(Equal(0))

		result = podmanTest.Podman([]string{"ps", "-q"})
		result.WaitWithDefaultTimeout()
		Expect(result.OutputToString()).To(BeEmpty())

		// one pod should have been deleted
		result = podmanTest.Podman([]string{"pod", "ps", "-q"})
		result.WaitWithDefaultTimeout()
		Expect(result.OutputToString()).To(BeEmpty())
	})
})
