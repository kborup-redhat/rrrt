package types

const (
	LabelOwner        = "rightsizing.redhatconsulting.io/owner"
	AnnotationExclude = "rightsizing.redhatconsulting.io/exclude"

	DefaultPrometheusURL   = "https://thanos-querier.openshift-monitoring.svc:9091"
	DefaultLookbackDays    = 30
	DefaultPercentile      = 95
	DefaultHeadroom        = 20
	DefaultUpsizeThreshold = 90

	DefaultVMMinCPUSavings = 1000    // millicores (1 core)
	DefaultVMMinMemSavings = 1 << 30 // 1 GiB in bytes

	DefaultContainerMinCPUSavings = 250       // millicores (250m)
	DefaultContainerMinMemSavings = 256 << 20 // 256 Mi in bytes

	OVRONamespace          = "ovro-system"
	OVROVictoriaMetricsURL = "http://victoriametrics.ovro-system.svc:8428"
	OVROCRD                = "rightsizingrecommendations.rightsizing.redhatconsulting.io"
)

type Direction string

const (
	Downsize Direction = "downsize"
	Upsize   Direction = "upsize"
)

type ResourceKind string

const (
	KindVM          ResourceKind = "VirtualMachine"
	KindDeployment  ResourceKind = "Deployment"
	KindStatefulSet ResourceKind = "StatefulSet"
)

type ResourceAnalysis struct {
	Namespace      string
	Name           string
	Kind           ResourceKind
	Owner          string
	ConsoleURL     string
	Direction      Direction
	CurrentCPU     int64 // millicores
	CurrentMem     int64 // bytes
	RecommendedCPU int64
	RecommendedMem int64
	CPUSavings     int64
	MemSavings     int64
	CPUP95         float64
	MemP95         float64
	CPUMax         float64
	MemMax         float64
	CPUSamples     []float64 // raw time-series for charts
	MemSamples     []float64
	Justification  string
}

type InsufficientDataEntry struct {
	Namespace      string
	Name           string
	Kind           ResourceKind
	DataPoints     int
	ExpectedPoints int
}

type ClusterOverview struct {
	TotalNodes  int
	ReadyNodes  int
	MasterNodes int
	WorkerNodes int

	CPUCapacity    int64 // millicores
	CPUAllocatable int64
	CPURequested   int64
	CPUUsed        int64

	MemCapacity    int64 // bytes
	MemAllocatable int64
	MemRequested   int64
	MemUsed        int64

	StorageRequested int64 // bytes
	StorageCapacity  int64
}

type ReportData struct {
	ClusterName       string
	ClusterID         string
	GeneratedAt       string
	Scope             string
	LookbackDays      int
	Percentile        int
	HeadroomPct       int
	ClusterOverview   *ClusterOverview
	VMAnalyses        []ResourceAnalysis
	ContainerAnalyses []ResourceAnalysis
	InsufficientData  []InsufficientDataEntry
	CLIVersion        string
	ImageVersion      string
	DataSource        string
}

type AnalyzerConfig struct {
	Namespaces        []string
	LookbackDays      int
	ConsoleURL        string
	IncludeOpenShift  bool
	PrometheusURL     string
	NoOVRO            bool
	OVROEndpoint      string
	AnalyzerNamespace string
}
