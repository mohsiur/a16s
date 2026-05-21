package view

type kind int

const (
	ClusterKind kind = iota
	ServiceKind
	TaskKind
	InstanceKind
	ContainerKind
	TaskDefinitionKind
	HelpKind
	DescriptionKind
	ServiceEventsKind
	ServiceDeploymentKind
	LogKind
	AutoScalingKind
	ServiceRevisionKind
	ModalKind
	EmptyKind
	ProfileKind
	RegionKind
	LambdaKind
	SQSKind
	SQSPeekKind
	DynamoDBKind
	DynamoDBIndexKind
	DynamoDBScanKind
	LambdaInvokeKind
	LambdaConfigKind
	LambdaLogTailKind
)

func (k kind) String() string {
	switch k {
	case ClusterKind:
		return "clusters"
	case ServiceKind:
		return "services"
	case TaskKind:
		return "tasks"
	case ContainerKind:
		return "containers"
	case HelpKind:
		return "help"
	case DescriptionKind:
		return "description"
	case TaskDefinitionKind:
		return "task definitions"
	case InstanceKind:
		return "instances"
	case ServiceEventsKind:
		return "service events"
	case ServiceDeploymentKind:
		return "service deployments"
	case ServiceRevisionKind:
		return "service revision"
	case LogKind:
		return "logs"
	case AutoScalingKind:
		return "autoscaling"
	case ModalKind:
		return "modal"
	case ProfileKind:
		return "profiles"
	case RegionKind:
		return "regions"
	case LambdaKind:
		return "lambdas"
	case SQSKind:
		return "queues"
	case SQSPeekKind:
		return "messages"
	case DynamoDBKind:
		return "tables"
	case DynamoDBIndexKind:
		return "indexes"
	case DynamoDBScanKind:
		return "items"
	case LambdaInvokeKind:
		return "invoke"
	case LambdaConfigKind:
		return "config"
	case LambdaLogTailKind:
		return "log tail"
	default:
		return "unknownKind"
	}
}

func (k kind) nextKind() kind {
	switch k {
	case ClusterKind:
		return ServiceKind
	case ServiceKind:
		return TaskKind
	case TaskKind:
		return ContainerKind
	case ContainerKind:
		return ContainerKind
	case SQSKind:
		return SQSPeekKind
	case DynamoDBKind:
		return DynamoDBIndexKind
	case DynamoDBIndexKind:
		return DynamoDBScanKind
	default:
		return ClusterKind
	}
}

func (k kind) prevKind() kind {
	switch k {
	case ClusterKind:
		return ProfileKind
	case InstanceKind:
		return ClusterKind
	case ProfileKind:
		return ProfileKind
	case RegionKind:
		return RegionKind
	case ServiceKind:
		return ClusterKind
	case TaskKind, TaskDefinitionKind, ServiceDeploymentKind:
		return ServiceKind
	case ContainerKind:
		return TaskKind
	case LambdaKind, SQSKind, DynamoDBKind:
		return ProfileKind
	case SQSPeekKind:
		return SQSKind
	case DynamoDBIndexKind:
		return DynamoDBKind
	case DynamoDBScanKind:
		return DynamoDBIndexKind
	default:
		return ProfileKind
	}
}

// isFlatLeaf reports whether the kind is a flat (non-ECS) leaf table whose
// rows can have many wide columns the user wants to scroll through with the
// arrow keys. On these views the arrow keys fall through to tview so it can
// move the column offset; h/Esc still navigate back.
func (k kind) isFlatLeaf() bool {
	switch k {
	case LambdaKind, SQSPeekKind, DynamoDBScanKind:
		return true
	default:
		return false
	}
}

// App page name is kind string + "." + cluster arn
func (k kind) getAppPageName(name string) string {
	prefix := globalProfile + "." + globalRegion
	switch k {
	case ProfileKind, RegionKind:
		return k.String()
	case ClusterKind, LambdaKind, SQSKind, DynamoDBKind:
		return prefix + "." + k.String()
	case ServiceKind, TaskKind, ContainerKind, TaskDefinitionKind, ServiceDeploymentKind, DescriptionKind, InstanceKind, SQSPeekKind, DynamoDBIndexKind, DynamoDBScanKind:
		return prefix + "." + k.String() + "." + name
	default:
		return prefix + "." + k.String()
	}
}

func (k kind) getTablePageName(name string) string {
	pageName := k.getAppPageName(name)
	return pageName + ".table"
}

func (k kind) getSecondaryPageName(name string) string {
	pageName := k.getAppPageName(name)
	return pageName + "." + DescriptionKind.String()
}
