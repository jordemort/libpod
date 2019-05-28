package shared

import (
	"context"
	"fmt"
	"io"
	v1 "k8s.io/api/core/v1"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containers/image/types"
	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/libpod/image"
	"github.com/containers/libpod/pkg/inspect"
	cc "github.com/containers/libpod/pkg/spec"
	"github.com/containers/libpod/pkg/util"
	"github.com/cri-o/ocicni/pkg/ocicni"
	"github.com/docker/go-units"
	"github.com/google/shlex"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	cidTruncLength = 12
	podTruncLength = 12
	cmdTruncLength = 17
)

// PsOptions describes the struct being formed for ps
type PsOptions struct {
	All       bool
	Format    string
	Last      int
	Latest    bool
	NoTrunc   bool
	Pod       bool
	Quiet     bool
	Size      bool
	Sort      string
	Namespace bool
	Sync      bool
}

// BatchContainerStruct is the return obkect from BatchContainer and contains
// container related information
type BatchContainerStruct struct {
	ConConfig   *libpod.ContainerConfig
	ConState    libpod.ContainerStatus
	ExitCode    int32
	Exited      bool
	Pid         int
	StartedTime time.Time
	ExitedTime  time.Time
	Size        *ContainerSize
}

// PsContainerOutput is the struct being returned from a parallel
// Batch operation
type PsContainerOutput struct {
	ID        string
	Image     string
	Command   string
	Created   string
	Ports     string
	Names     string
	IsInfra   bool
	Status    string
	State     libpod.ContainerStatus
	Pid       int
	Size      *ContainerSize
	Pod       string
	CreatedAt time.Time
	ExitedAt  time.Time
	StartedAt time.Time
	Labels    map[string]string
	PID       string
	Cgroup    string
	IPC       string
	MNT       string
	NET       string
	PIDNS     string
	User      string
	UTS       string
	Mounts    string
}

// Namespace describes output for ps namespace
type Namespace struct {
	PID    string `json:"pid,omitempty"`
	Cgroup string `json:"cgroup,omitempty"`
	IPC    string `json:"ipc,omitempty"`
	MNT    string `json:"mnt,omitempty"`
	NET    string `json:"net,omitempty"`
	PIDNS  string `json:"pidns,omitempty"`
	User   string `json:"user,omitempty"`
	UTS    string `json:"uts,omitempty"`
}

// ContainerSize holds the size of the container's root filesystem and top
// read-write layer
type ContainerSize struct {
	RootFsSize int64 `json:"rootFsSize"`
	RwSize     int64 `json:"rwSize"`
}

// NewBatchContainer runs a batch process under one lock to get container information and only
// be called in PBatch
func NewBatchContainer(ctr *libpod.Container, opts PsOptions) (PsContainerOutput, error) {
	var (
		conState  libpod.ContainerStatus
		command   string
		created   string
		status    string
		exitedAt  time.Time
		startedAt time.Time
		exitCode  int32
		err       error
		pid       int
		size      *ContainerSize
		ns        *Namespace
		pso       PsContainerOutput
	)
	batchErr := ctr.Batch(func(c *libpod.Container) error {
		if opts.Sync {
			if err := c.Sync(); err != nil {
				return err
			}
		}

		conState, err = c.State()
		if err != nil {
			return errors.Wrapf(err, "unable to obtain container state")
		}
		command = strings.Join(c.Command(), " ")
		created = units.HumanDuration(time.Since(c.CreatedTime())) + " ago"

		exitCode, _, err = c.ExitCode()
		if err != nil {
			return errors.Wrapf(err, "unable to obtain container exit code")
		}
		startedAt, err = c.StartedTime()
		if err != nil {
			logrus.Errorf("error getting started time for %q: %v", c.ID(), err)
		}
		exitedAt, err = c.FinishedTime()
		if err != nil {
			logrus.Errorf("error getting exited time for %q: %v", c.ID(), err)
		}
		if opts.Namespace {
			pid, err = c.PID()
			if err != nil {
				return errors.Wrapf(err, "unable to obtain container pid")
			}
			ns = GetNamespaces(pid)
		}
		if opts.Size {
			size = new(ContainerSize)

			rootFsSize, err := c.RootFsSize()
			if err != nil {
				logrus.Errorf("error getting root fs size for %q: %v", c.ID(), err)
			}

			rwSize, err := c.RWSize()
			if err != nil {
				logrus.Errorf("error getting rw size for %q: %v", c.ID(), err)
			}

			size.RootFsSize = rootFsSize
			size.RwSize = rwSize
		}

		return nil
	})

	if batchErr != nil {
		return pso, batchErr
	}

	switch conState.String() {
	case libpod.ContainerStateExited.String():
		fallthrough
	case libpod.ContainerStateStopped.String():
		exitedSince := units.HumanDuration(time.Since(exitedAt))
		status = fmt.Sprintf("Exited (%d) %s ago", exitCode, exitedSince)
	case libpod.ContainerStateRunning.String():
		status = "Up " + units.HumanDuration(time.Since(startedAt)) + " ago"
	case libpod.ContainerStatePaused.String():
		status = "Paused"
	case libpod.ContainerStateCreated.String(), libpod.ContainerStateConfigured.String():
		status = "Created"
	default:
		status = "Error"
	}

	_, imageName := ctr.Image()
	cid := ctr.ID()
	pod := ctr.PodID()
	if !opts.NoTrunc {
		cid = cid[0:cidTruncLength]
		if len(pod) > podTruncLength {
			pod = pod[0:podTruncLength]
		}
		if len(command) > cmdTruncLength {
			command = command[0:cmdTruncLength] + "..."
		}
	}

	ports, err := ctr.PortMappings()
	if err != nil {
		logrus.Errorf("unable to lookup namespace container for %s", ctr.ID())
	}

	pso.ID = cid
	pso.Image = imageName
	pso.Command = command
	pso.Created = created
	pso.Ports = portsToString(ports)
	pso.Names = ctr.Name()
	pso.IsInfra = ctr.IsInfra()
	pso.Status = status
	pso.State = conState
	pso.Pid = pid
	pso.Size = size
	pso.Pod = pod
	pso.ExitedAt = exitedAt
	pso.CreatedAt = ctr.CreatedTime()
	pso.StartedAt = startedAt
	pso.Labels = ctr.Labels()
	pso.Mounts = strings.Join(ctr.UserVolumes(), " ")

	if opts.Namespace {
		pso.Cgroup = ns.Cgroup
		pso.IPC = ns.IPC
		pso.MNT = ns.MNT
		pso.NET = ns.NET
		pso.User = ns.User
		pso.UTS = ns.UTS
		pso.PIDNS = ns.PIDNS
	}

	return pso, nil
}

type batchFunc func() (PsContainerOutput, error)

type workerInput struct {
	parallelFunc batchFunc
	opts         PsOptions
	cid          string
	job          int
}

// worker is a "threaded" worker that takes jobs from the channel "queue"
func worker(wg *sync.WaitGroup, jobs <-chan workerInput, results chan<- PsContainerOutput, errors chan<- error) {
	for j := range jobs {
		r, err := j.parallelFunc()
		// If we find an error, we return just the error
		if err != nil {
			errors <- err
		} else {
			// Return the result
			results <- r
		}
		wg.Done()
	}
}

func generateContainerFilterFuncs(filter, filterValue string, r *libpod.Runtime) (func(container *libpod.Container) bool, error) {
	switch filter {
	case "id":
		return func(c *libpod.Container) bool {
			return strings.Contains(c.ID(), filterValue)
		}, nil
	case "label":
		var filterArray []string = strings.SplitN(filterValue, "=", 2)
		var filterKey string = filterArray[0]
		if len(filterArray) > 1 {
			filterValue = filterArray[1]
		} else {
			filterValue = ""
		}
		return func(c *libpod.Container) bool {
			for labelKey, labelValue := range c.Labels() {
				if labelKey == filterKey && ("" == filterValue || labelValue == filterValue) {
					return true
				}
			}
			return false
		}, nil
	case "name":
		return func(c *libpod.Container) bool {
			return strings.Contains(c.Name(), filterValue)
		}, nil
	case "exited":
		exitCode, err := strconv.ParseInt(filterValue, 10, 32)
		if err != nil {
			return nil, errors.Wrapf(err, "exited code out of range %q", filterValue)
		}
		return func(c *libpod.Container) bool {
			ec, exited, err := c.ExitCode()
			if ec == int32(exitCode) && err == nil && exited == true {
				return true
			}
			return false
		}, nil
	case "status":
		if !util.StringInSlice(filterValue, []string{"created", "running", "paused", "stopped", "exited", "unknown"}) {
			return nil, errors.Errorf("%s is not a valid status", filterValue)
		}
		return func(c *libpod.Container) bool {
			status, err := c.State()
			if err != nil {
				return false
			}
			if filterValue == "stopped" {
				filterValue = "exited"
			}
			state := status.String()
			if status == libpod.ContainerStateConfigured {
				state = "created"
			} else if status == libpod.ContainerStateStopped {
				state = "exited"
			}
			return state == filterValue
		}, nil
	case "ancestor":
		// This needs to refine to match docker
		// - ancestor=(<image-name>[:tag]|<image-id>| ⟨image@digest⟩) - containers created from an image or a descendant.
		return func(c *libpod.Container) bool {
			containerConfig := c.Config()
			if strings.Contains(containerConfig.RootfsImageID, filterValue) || strings.Contains(containerConfig.RootfsImageName, filterValue) {
				return true
			}
			return false
		}, nil
	case "before":
		ctr, err := r.LookupContainer(filterValue)
		if err != nil {
			return nil, errors.Errorf("unable to find container by name or id of %s", filterValue)
		}
		containerConfig := ctr.Config()
		createTime := containerConfig.CreatedTime
		return func(c *libpod.Container) bool {
			cc := c.Config()
			return createTime.After(cc.CreatedTime)
		}, nil
	case "since":
		ctr, err := r.LookupContainer(filterValue)
		if err != nil {
			return nil, errors.Errorf("unable to find container by name or id of %s", filterValue)
		}
		containerConfig := ctr.Config()
		createTime := containerConfig.CreatedTime
		return func(c *libpod.Container) bool {
			cc := c.Config()
			return createTime.Before(cc.CreatedTime)
		}, nil
	case "volume":
		//- volume=(<volume-name>|<mount-point-destination>)
		return func(c *libpod.Container) bool {
			containerConfig := c.Config()
			var dest string
			arr := strings.Split(filterValue, ":")
			source := arr[0]
			if len(arr) == 2 {
				dest = arr[1]
			}
			for _, mount := range containerConfig.Spec.Mounts {
				if dest != "" && (mount.Source == source && mount.Destination == dest) {
					return true
				}
				if dest == "" && mount.Source == source {
					return true
				}
			}
			return false
		}, nil
	case "health":
		return func(c *libpod.Container) bool {
			hcStatus, err := c.HealthCheckStatus()
			if err != nil {
				return false
			}
			return hcStatus == filterValue
		}, nil
	}
	return nil, errors.Errorf("%s is an invalid filter", filter)
}

// GetPsContainerOutput returns a slice of containers specifically for ps output
func GetPsContainerOutput(r *libpod.Runtime, opts PsOptions, filters []string, maxWorkers int) ([]PsContainerOutput, error) {
	var (
		filterFuncs      []libpod.ContainerFilter
		outputContainers []*libpod.Container
	)

	if len(filters) > 0 {
		for _, f := range filters {
			filterSplit := strings.SplitN(f, "=", 2)
			if len(filterSplit) < 2 {
				return nil, errors.Errorf("filter input must be in the form of filter=value: %s is invalid", f)
			}
			generatedFunc, err := generateContainerFilterFuncs(filterSplit[0], filterSplit[1], r)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid filter")
			}
			filterFuncs = append(filterFuncs, generatedFunc)
		}
	}
	if !opts.Latest {
		// Get all containers
		containers, err := r.GetContainers(filterFuncs...)
		if err != nil {
			return nil, err
		}

		// We only want the last few containers
		if opts.Last > 0 && opts.Last <= len(containers) {
			return nil, errors.Errorf("--last not yet supported")
		} else {
			outputContainers = containers
		}
	} else {
		// Get just the latest container
		// Ignore filters
		latestCtr, err := r.GetLatestContainer()
		if err != nil {
			return nil, err
		}

		outputContainers = []*libpod.Container{latestCtr}
	}

	pss := PBatch(outputContainers, maxWorkers, opts)
	return pss, nil
}

// PBatch is performs batch operations on a container in parallel. It spawns the number of workers
// relative to the the number of parallel operations desired.
func PBatch(containers []*libpod.Container, workers int, opts PsOptions) []PsContainerOutput {
	var (
		wg        sync.WaitGroup
		psResults []PsContainerOutput
	)

	// If the number of containers in question is less than the number of
	// proposed parallel operations, we shouldnt spawn so many workers
	if workers > len(containers) {
		workers = len(containers)
	}

	jobs := make(chan workerInput, len(containers))
	results := make(chan PsContainerOutput, len(containers))
	batchErrors := make(chan error, len(containers))

	// Create the workers
	for w := 1; w <= workers; w++ {
		go worker(&wg, jobs, results, batchErrors)
	}

	// Add jobs to the workers
	for i, j := range containers {
		j := j
		wg.Add(1)
		f := func() (PsContainerOutput, error) {
			return NewBatchContainer(j, opts)
		}
		jobs <- workerInput{
			parallelFunc: f,
			opts:         opts,
			cid:          j.ID(),
			job:          i,
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	close(batchErrors)
	for err := range batchErrors {
		logrus.Errorf("unable to get container info: %q", err)
	}
	for res := range results {
		// We sort out running vs non-running here to save lots of copying
		// later.
		if !opts.All && !opts.Latest && opts.Last < 1 {
			if !res.IsInfra && res.State == libpod.ContainerStateRunning {
				psResults = append(psResults, res)
			}
		} else {
			psResults = append(psResults, res)
		}
	}
	return psResults
}

// BatchContainer is used in ps to reduce performance hits by "batching"
// locks.
func BatchContainerOp(ctr *libpod.Container, opts PsOptions) (BatchContainerStruct, error) {
	var (
		conConfig   *libpod.ContainerConfig
		conState    libpod.ContainerStatus
		err         error
		exitCode    int32
		exited      bool
		pid         int
		size        *ContainerSize
		startedTime time.Time
		exitedTime  time.Time
	)

	batchErr := ctr.Batch(func(c *libpod.Container) error {
		conConfig = c.Config()
		conState, err = c.State()
		if err != nil {
			return errors.Wrapf(err, "unable to obtain container state")
		}

		exitCode, exited, err = c.ExitCode()
		if err != nil {
			return errors.Wrapf(err, "unable to obtain container exit code")
		}
		startedTime, err = c.StartedTime()
		if err != nil {
			logrus.Errorf("error getting started time for %q: %v", c.ID(), err)
		}
		exitedTime, err = c.FinishedTime()
		if err != nil {
			logrus.Errorf("error getting exited time for %q: %v", c.ID(), err)
		}

		if !opts.Size && !opts.Namespace {
			return nil
		}

		if opts.Namespace {
			pid, err = c.PID()
			if err != nil {
				return errors.Wrapf(err, "unable to obtain container pid")
			}
		}
		if opts.Size {
			size = new(ContainerSize)

			rootFsSize, err := c.RootFsSize()
			if err != nil {
				logrus.Errorf("error getting root fs size for %q: %v", c.ID(), err)
			}

			rwSize, err := c.RWSize()
			if err != nil {
				logrus.Errorf("error getting rw size for %q: %v", c.ID(), err)
			}

			size.RootFsSize = rootFsSize
			size.RwSize = rwSize
		}
		return nil
	})
	if batchErr != nil {
		return BatchContainerStruct{}, batchErr
	}
	return BatchContainerStruct{
		ConConfig:   conConfig,
		ConState:    conState,
		ExitCode:    exitCode,
		Exited:      exited,
		Pid:         pid,
		StartedTime: startedTime,
		ExitedTime:  exitedTime,
		Size:        size,
	}, nil
}

// GetNamespaces returns a populated namespace struct
func GetNamespaces(pid int) *Namespace {
	ctrPID := strconv.Itoa(pid)
	cgroup, _ := getNamespaceInfo(filepath.Join("/proc", ctrPID, "ns", "cgroup"))
	ipc, _ := getNamespaceInfo(filepath.Join("/proc", ctrPID, "ns", "ipc"))
	mnt, _ := getNamespaceInfo(filepath.Join("/proc", ctrPID, "ns", "mnt"))
	net, _ := getNamespaceInfo(filepath.Join("/proc", ctrPID, "ns", "net"))
	pidns, _ := getNamespaceInfo(filepath.Join("/proc", ctrPID, "ns", "pid"))
	user, _ := getNamespaceInfo(filepath.Join("/proc", ctrPID, "ns", "user"))
	uts, _ := getNamespaceInfo(filepath.Join("/proc", ctrPID, "ns", "uts"))

	return &Namespace{
		PID:    ctrPID,
		Cgroup: cgroup,
		IPC:    ipc,
		MNT:    mnt,
		NET:    net,
		PIDNS:  pidns,
		User:   user,
		UTS:    uts,
	}
}

func getNamespaceInfo(path string) (string, error) {
	val, err := os.Readlink(path)
	if err != nil {
		return "", errors.Wrapf(err, "error getting info from %q", path)
	}
	return getStrFromSquareBrackets(val), nil
}

// getStrFromSquareBrackets gets the string inside [] from a string
func getStrFromSquareBrackets(cmd string) string {
	reg, err := regexp.Compile(".*\\[|\\].*")
	if err != nil {
		return ""
	}
	arr := strings.Split(reg.ReplaceAllLiteralString(cmd, ""), ",")
	return strings.Join(arr, ",")
}

// GetCtrInspectInfo takes container inspect data and collects all its info into a ContainerData
// structure for inspection related methods
func GetCtrInspectInfo(config *libpod.ContainerConfig, ctrInspectData *inspect.ContainerInspectData, createArtifact *cc.CreateConfig) (*inspect.ContainerData, error) {
	spec := config.Spec

	cpus, mems, period, quota, realtimePeriod, realtimeRuntime, shares := getCPUInfo(spec)
	blkioWeight, blkioWeightDevice, blkioReadBps, blkioWriteBps, blkioReadIOPS, blkioeWriteIOPS := getBLKIOInfo(spec)
	memKernel, memReservation, memSwap, memSwappiness, memDisableOOMKiller := getMemoryInfo(spec)
	pidsLimit := getPidsInfo(spec)
	cgroup := getCgroup(spec)

	data := &inspect.ContainerData{
		ctrInspectData,
		&inspect.HostConfig{
			ConsoleSize:          spec.Process.ConsoleSize,
			OomScoreAdj:          spec.Process.OOMScoreAdj,
			CPUShares:            shares,
			BlkioWeight:          blkioWeight,
			BlkioWeightDevice:    blkioWeightDevice,
			BlkioDeviceReadBps:   blkioReadBps,
			BlkioDeviceWriteBps:  blkioWriteBps,
			BlkioDeviceReadIOps:  blkioReadIOPS,
			BlkioDeviceWriteIOps: blkioeWriteIOPS,
			CPUPeriod:            period,
			CPUQuota:             quota,
			CPURealtimePeriod:    realtimePeriod,
			CPURealtimeRuntime:   realtimeRuntime,
			CPUSetCPUs:           cpus,
			CPUSetMems:           mems,
			Devices:              spec.Linux.Devices,
			KernelMemory:         memKernel,
			MemoryReservation:    memReservation,
			MemorySwap:           memSwap,
			MemorySwappiness:     memSwappiness,
			OomKillDisable:       memDisableOOMKiller,
			PidsLimit:            pidsLimit,
			Privileged:           config.Privileged,
			ReadOnlyRootfs:       spec.Root.Readonly,
			ReadOnlyTmpfs:        createArtifact.ReadOnlyTmpfs,
			Runtime:              config.OCIRuntime,
			NetworkMode:          string(createArtifact.NetMode),
			IpcMode:              string(createArtifact.IpcMode),
			Cgroup:               cgroup,
			UTSMode:              string(createArtifact.UtsMode),
			UsernsMode:           string(createArtifact.UsernsMode),
			GroupAdd:             spec.Process.User.AdditionalGids,
			ContainerIDFile:      createArtifact.CidFile,
			AutoRemove:           createArtifact.Rm,
			CapAdd:               createArtifact.CapAdd,
			CapDrop:              createArtifact.CapDrop,
			DNS:                  createArtifact.DNSServers,
			DNSOptions:           createArtifact.DNSOpt,
			DNSSearch:            createArtifact.DNSSearch,
			PidMode:              string(createArtifact.PidMode),
			CgroupParent:         createArtifact.CgroupParent,
			ShmSize:              createArtifact.Resources.ShmSize,
			Memory:               createArtifact.Resources.Memory,
			Ulimits:              createArtifact.Resources.Ulimit,
			SecurityOpt:          createArtifact.SecurityOpts,
			Tmpfs:                createArtifact.Tmpfs,
		},
		&inspect.CtrConfig{
			Hostname:    spec.Hostname,
			User:        spec.Process.User,
			Env:         spec.Process.Env,
			Image:       config.RootfsImageName,
			WorkingDir:  spec.Process.Cwd,
			Labels:      config.Labels,
			Annotations: spec.Annotations,
			Tty:         spec.Process.Terminal,
			OpenStdin:   config.Stdin,
			StopSignal:  config.StopSignal,
			Cmd:         config.Spec.Process.Args,
			Entrypoint:  strings.Join(createArtifact.Entrypoint, " "),
			Healthcheck: config.HealthCheckConfig,
		},
	}
	return data, nil
}

func getCPUInfo(spec *specs.Spec) (string, string, *uint64, *int64, *uint64, *int64, *uint64) {
	if spec.Linux.Resources == nil {
		return "", "", nil, nil, nil, nil, nil
	}
	cpu := spec.Linux.Resources.CPU
	if cpu == nil {
		return "", "", nil, nil, nil, nil, nil
	}
	return cpu.Cpus, cpu.Mems, cpu.Period, cpu.Quota, cpu.RealtimePeriod, cpu.RealtimeRuntime, cpu.Shares
}

func getBLKIOInfo(spec *specs.Spec) (*uint16, []specs.LinuxWeightDevice, []specs.LinuxThrottleDevice, []specs.LinuxThrottleDevice, []specs.LinuxThrottleDevice, []specs.LinuxThrottleDevice) {
	if spec.Linux.Resources == nil {
		return nil, nil, nil, nil, nil, nil
	}
	blkio := spec.Linux.Resources.BlockIO
	if blkio == nil {
		return nil, nil, nil, nil, nil, nil
	}
	return blkio.Weight, blkio.WeightDevice, blkio.ThrottleReadBpsDevice, blkio.ThrottleWriteBpsDevice, blkio.ThrottleReadIOPSDevice, blkio.ThrottleWriteIOPSDevice
}

func getMemoryInfo(spec *specs.Spec) (*int64, *int64, *int64, *uint64, *bool) {
	if spec.Linux.Resources == nil {
		return nil, nil, nil, nil, nil
	}
	memory := spec.Linux.Resources.Memory
	if memory == nil {
		return nil, nil, nil, nil, nil
	}
	return memory.Kernel, memory.Reservation, memory.Swap, memory.Swappiness, memory.DisableOOMKiller
}

func getPidsInfo(spec *specs.Spec) *int64 {
	if spec.Linux.Resources == nil {
		return nil
	}
	pids := spec.Linux.Resources.Pids
	if pids == nil {
		return nil
	}
	return &pids.Limit
}

func getCgroup(spec *specs.Spec) string {
	cgroup := "host"
	for _, ns := range spec.Linux.Namespaces {
		if ns.Type == specs.CgroupNamespace && ns.Path != "" {
			cgroup = "container"
		}
	}
	return cgroup
}

func comparePorts(i, j ocicni.PortMapping) bool {
	if i.ContainerPort != j.ContainerPort {
		return i.ContainerPort < j.ContainerPort
	}

	if i.HostIP != j.HostIP {
		return i.HostIP < j.HostIP
	}

	if i.HostPort != j.HostPort {
		return i.HostPort < j.HostPort
	}

	return i.Protocol < j.Protocol
}

// returns the group as <IP:startPort:lastPort->startPort:lastPort/Proto>
// e.g 0.0.0.0:1000-1006->1000-1006/tcp
func formatGroup(key string, start, last int32) string {
	parts := strings.Split(key, "/")
	groupType := parts[0]
	var ip string
	if len(parts) > 1 {
		ip = parts[0]
		groupType = parts[1]
	}
	group := strconv.Itoa(int(start))
	if start != last {
		group = fmt.Sprintf("%s-%d", group, last)
	}
	if ip != "" {
		group = fmt.Sprintf("%s:%s->%s", ip, group, group)
	}
	return fmt.Sprintf("%s/%s", group, groupType)
}

// portsToString converts the ports used to a string of the from "port1, port2"
// also groups continuous list of ports in readable format.
func portsToString(ports []ocicni.PortMapping) string {
	type portGroup struct {
		first int32
		last  int32
	}
	var portDisplay []string
	if len(ports) == 0 {
		return ""
	}
	//Sort the ports, so grouping continuous ports become easy.
	sort.Slice(ports, func(i, j int) bool {
		return comparePorts(ports[i], ports[j])
	})

	// portGroupMap is used for grouping continuous ports
	portGroupMap := make(map[string]*portGroup)
	var groupKeyList []string

	for _, v := range ports {

		hostIP := v.HostIP
		if hostIP == "" {
			hostIP = "0.0.0.0"
		}
		// if hostPort and containerPort are not same, consider as individual port.
		if v.ContainerPort != v.HostPort {
			portDisplay = append(portDisplay, fmt.Sprintf("%s:%d->%d/%s", hostIP, v.HostPort, v.ContainerPort, v.Protocol))
			continue
		}

		portMapKey := fmt.Sprintf("%s/%s", hostIP, v.Protocol)

		portgroup, ok := portGroupMap[portMapKey]
		if !ok {
			portGroupMap[portMapKey] = &portGroup{first: v.ContainerPort, last: v.ContainerPort}
			// this list is required to travese portGroupMap
			groupKeyList = append(groupKeyList, portMapKey)
			continue
		}

		if portgroup.last == (v.ContainerPort - 1) {
			portgroup.last = v.ContainerPort
			continue
		}
	}
	// for each portMapKey, format group list and appned to output string
	for _, portKey := range groupKeyList {
		group := portGroupMap[portKey]
		portDisplay = append(portDisplay, formatGroup(portKey, group.first, group.last))
	}
	return strings.Join(portDisplay, ", ")
}

// GetRunlabel is a helper function for runlabel; it gets the image if needed and begins the
// contruction of the runlabel output and environment variables
func GetRunlabel(label string, runlabelImage string, ctx context.Context, runtime *libpod.Runtime, pull bool, inputCreds string, dockerRegistryOptions image.DockerRegistryOptions, authfile string, signaturePolicyPath string, output io.Writer) (string, string, error) {
	var (
		newImage  *image.Image
		err       error
		imageName string
	)
	if pull {
		var registryCreds *types.DockerAuthConfig
		if inputCreds != "" {
			creds, err := util.ParseRegistryCreds(inputCreds)
			if err != nil {
				return "", "", err
			}
			registryCreds = creds
		}
		dockerRegistryOptions.DockerRegistryCreds = registryCreds
		newImage, err = runtime.ImageRuntime().New(ctx, runlabelImage, signaturePolicyPath, authfile, output, &dockerRegistryOptions, image.SigningOptions{}, false, &label)
	} else {
		newImage, err = runtime.ImageRuntime().NewFromLocal(runlabelImage)
	}
	if err != nil {
		return "", "", errors.Wrapf(err, "unable to find image")
	}

	if len(newImage.Names()) < 1 {
		imageName = newImage.ID()
	} else {
		imageName = newImage.Names()[0]
	}

	runLabel, err := newImage.GetLabel(ctx, label)
	return runLabel, imageName, err
}

// GenerateRunlabelCommand generates the command that will eventually be execucted by podman
func GenerateRunlabelCommand(runLabel, imageName, name string, opts map[string]string, extraArgs []string, globalOpts string) ([]string, []string, error) {
	// If no name is provided, we use the image's basename instead
	if name == "" {
		baseName, err := image.GetImageBaseName(imageName)
		if err != nil {
			return nil, nil, err
		}
		name = baseName
	}
	// The user provided extra arguments that need to be tacked onto the label's command
	if len(extraArgs) > 0 {
		runLabel = fmt.Sprintf("%s %s", runLabel, strings.Join(extraArgs, " "))
	}
	cmd, err := GenerateCommand(runLabel, imageName, name, globalOpts)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to generate command")
	}
	env := GenerateRunEnvironment(name, imageName, opts)
	env = append(env, "PODMAN_RUNLABEL_NESTED=1")

	envmap := envSliceToMap(env)

	envmapper := func(k string) string {
		switch k {
		case "OPT1":
			return envmap["OPT1"]
		case "OPT2":
			return envmap["OPT2"]
		case "OPT3":
			return envmap["OPT3"]
		case "PWD":
			// I would prefer to use os.getenv but it appears PWD is not in the os env list
			d, err := os.Getwd()
			if err != nil {
				logrus.Error("unable to determine current working directory")
				return ""
			}
			return d
		}
		return ""
	}
	newS := os.Expand(strings.Join(cmd, " "), envmapper)
	cmd, err = shlex.Split(newS)
	if err != nil {
		return nil, nil, err
	}
	return cmd, env, nil
}

func envSliceToMap(env []string) map[string]string {
	m := make(map[string]string)
	for _, i := range env {
		split := strings.Split(i, "=")
		m[split[0]] = strings.Join(split[1:], " ")
	}
	return m
}

// GenerateKube generates kubernetes yaml based on a pod or container
func GenerateKube(name string, service bool, r *libpod.Runtime) (*v1.Pod, *v1.Service, error) {
	var (
		pod          *libpod.Pod
		podYAML      *v1.Pod
		err          error
		container    *libpod.Container
		servicePorts []v1.ServicePort
		serviceYAML  v1.Service
	)
	// Get the container in question
	container, err = r.LookupContainer(name)
	if err != nil {
		pod, err = r.LookupPod(name)
		if err != nil {
			return nil, nil, err
		}
		podYAML, servicePorts, err = pod.GenerateForKube()
	} else {
		if len(container.Dependencies()) > 0 {
			return nil, nil, errors.Wrapf(libpod.ErrNotImplemented, "containers with dependencies")
		}
		podYAML, err = container.GenerateForKube()
	}
	if err != nil {
		return nil, nil, err
	}

	if service {
		serviceYAML = libpod.GenerateKubeServiceFromV1Pod(podYAML, servicePorts)
	}
	return podYAML, &serviceYAML, nil
}
