package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mohsiur/a16s/internal/color"
)

const (
	EmptyText               = "<empty>"
	clusterFmt              = "https://%s.console.aws.amazon.com/ecs/v2/clusters/%s"
	regionFmt               = "?region=%s"
	serviceFmt              = "/services/%s"
	taskFmt                 = "/tasks/%s"
	clusterURLFmt           = clusterFmt + regionFmt
	serviceURLFmt           = clusterFmt + serviceFmt + regionFmt
	taskURLFmt              = clusterFmt + serviceFmt + taskFmt + regionFmt
	taskDefinitionURLFmt    = "https://%s.console.aws.amazon.com/ecs/v2/task-definitions/%s/%s/containers?region=%s"
	serviceDeploymentURLFmt = "https://%s.console.aws.amazon.com/ecs/v2/clusters/%s/services/%s/service-deployments/%s?region=%s"
)

func ArnToName(arn *string) string {
	if arn == nil {
		return EmptyText
	}
	ss := strings.Split(*arn, "/")
	return ss[len(ss)-1]
}

func ArnToFullName(arn *string) string {
	if arn == nil {
		return EmptyText
	}
	ss := strings.Split(*arn, ":")
	return ss[len(ss)-1]
}

func ShowString(s *string) string {
	if s == nil {
		return EmptyText
	}
	return *s
}

func ShowArray(s []string) string {
	if len(s) == 0 {
		return EmptyText
	}
	return strings.Join(s, ",")
}

func ShowTime(at *time.Time) string {
	if at == nil {
		return EmptyText
	}
	loc, err := time.LoadLocation("Local")
	if err != nil {
		return at.Format(time.RFC3339)
	}
	return at.In(loc).Format(time.RFC3339)
}

func ShowInt(p *int32) string {
	if p == nil {
		return EmptyText
	}
	return strconv.Itoa(int(*p))
}

func ShowGreenGrey(inputStr *string, greenStr string) string {
	if inputStr == nil {
		return EmptyText
	}

	str := *inputStr
	if str == "" {
		return EmptyText
	}
	outputStr := strings.ToUpper(string(str[0])) + strings.ToLower(str[1:])
	if strings.ToLower(str) == greenStr {
		return fmt.Sprintf(color.GreenFmt, outputStr)
	}
	return fmt.Sprintf(color.GrayFmt, outputStr)
}

// Convert ARN to AWS web console URL
// TaskARN not contains service but need service name as second argument
func ArnToUrl(arn string, taskService string) string {
	components := strings.Split(arn, ":")
	resources := components[len(components)-1]
	names := strings.Split(resources, "/")
	_, err := strconv.Atoi(resources)
	if err == nil {
		// it's a task definition arn
		resources := components[len(components)-2]
		names = strings.Split(resources, "/")
		names = append(names, components[len(components)-1])
	}

	region := components[3]
	clusterName := ""
	serviceName := ""
	taskName := ""

	switch names[0] {
	case "cluster":
		clusterName = names[1]
		return fmt.Sprintf(clusterURLFmt, region, clusterName, region)
	case "service":
		clusterName = names[1]
		serviceName = names[2]
		return fmt.Sprintf(serviceURLFmt, region, clusterName, serviceName, region)
	case "service-deployment":
		clusterName = names[1]
		serviceName = names[2]
		deploymentId := names[3]
		return fmt.Sprintf(serviceDeploymentURLFmt, region, clusterName, serviceName, deploymentId, region)
	case "task", "container":
		clusterName = names[1]
		taskName = names[2]
		return fmt.Sprintf(taskURLFmt, region, clusterName, taskService, taskName, region)
	case "task-definition":
		taskDefName := names[1]
		revision := names[2]
		return fmt.Sprintf(taskDefinitionURLFmt, region, taskDefName, revision, region)
	default:
		return ""
	}
}

// LambdaFunctionURL builds the AWS console URL for a Lambda function. Returns
// "" when region or fn are empty so callers can early-bail without panicking.
func LambdaFunctionURL(region, fn string) string {
	if region == "" || fn == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.console.aws.amazon.com/lambda/home?region=%s#/functions/%s", region, region, url.QueryEscape(fn))
}

// SQSQueueURL builds the AWS console URL for an SQS queue. The console expects
// the full queue URL, fully URL-encoded, after the `#/queues/` fragment.
func SQSQueueURL(region, queueURL string) string {
	if region == "" || queueURL == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.console.aws.amazon.com/sqs/v3/home?region=%s#/queues/%s", region, region, url.QueryEscape(queueURL))
}

// DynamoDBTableURL builds the AWS console URL for a DynamoDB table. The
// dynamodbv2 console keys the explorer view by table name in the fragment.
func DynamoDBTableURL(region, table string) string {
	if region == "" || table == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.console.aws.amazon.com/dynamodbv2/home?region=%s#table?name=%s", region, region, table)
}

// S3BucketURL builds the AWS console URL for an S3 bucket. S3 buckets are
// global, so the console URL is region-prefixed only for the host — the
// objects view itself is not regional. Returns "" when bucket is empty.
func S3BucketURL(region, bucket string) string {
	if bucket == "" {
		return ""
	}
	if region == "" {
		region = "us-east-1"
	}
	return fmt.Sprintf("https://%s.console.aws.amazon.com/s3/buckets/%s", region, url.QueryEscape(bucket))
}

func OpenURL(url string) error {
	var err error

	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}

	if err != nil {
		return err
	}
	return nil
}

func BuildMeterText(f float64) string {
	const yesBlock = "█"
	const noBlock = "▒"
	i := int(f)

	yesNum := i / 5
	if yesNum == 0 {
		yesNum++
	}
	noNum := 20 - yesNum
	meterVal := strings.Join([]string{
		strings.Repeat(yesBlock, yesNum),
		strings.Repeat(noBlock, noNum),
	}, "")

	return meterVal + " " + fmt.Sprintf("%.2f", f) + "%"
}

// ShowVersion returns the version string shown by `a16s --version`. It best-
// effort fetches the latest release from GitHub to surface an upgrade hint,
// but never blocks startup or crashes the CLI: any network/parse failure
// degrades to printing only the current AppVersion. Cobra calls this during
// command construction, so even `a16s --help` runs through here — failing
// fast here used to crash the app whenever GitHub was unreachable.
func ShowVersion() string {
	latestVersion, err := fetchLatestVersion()
	if err != nil {
		slog.Debug("version check failed", "error", err)
		return fmt.Sprintf("\nCurrent: %s", AppVersion)
	}

	message := ""
	if latestVersion != AppVersion {
		message = "\nPlease upgrade a16s to latest version on https://github.com/mohsiur/a16s/releases"
	}
	return fmt.Sprintf("\nCurrent: %s\nLatest: %s%s", AppVersion, latestVersion, message)
}

// versionCheckTimeout caps the GitHub round-trip during `--version`. Kept
// short because the call blocks cobra's command construction.
const versionCheckTimeout = 2 * time.Second

// latestReleaseURL is the GitHub releases endpoint hit by fetchLatestVersion.
// Kept as a package var so tests can point at a httptest.Server without
// touching the network.
var latestReleaseURL = "https://api.github.com/repos/mohsiur/a16s/releases/latest"

func fetchLatestVersion() (string, error) {
	type ghRes struct {
		Name string `json:"name"`
	}
	client := &http.Client{Timeout: versionCheckTimeout}
	resp, err := client.Get(latestReleaseURL)
	if err != nil {
		return "", fmt.Errorf("github request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	var rsp ghRes
	if err := json.Unmarshal(body, &rsp); err != nil {
		return "", fmt.Errorf("decode body: %w", err)
	}
	if rsp.Name == "" {
		return "", fmt.Errorf("empty release name")
	}
	return rsp.Name, nil
}

func Age(t *time.Time) string {
	if t == nil {
		return EmptyText
	}
	now := time.Now()
	if now.Before(*t) {
		return "0s"
	}
	duration := now.Sub(*t)

	if years := int(duration.Hours() / 24 / 365); years > 0 {
		return fmt.Sprintf("%dy ago", years)
	}
	if months := int(duration.Hours() / 24 / 30); months > 0 {
		return fmt.Sprintf("%dmo ago", months)
	}
	if weeks := int(duration.Hours() / 24 / 7); weeks > 0 {
		return fmt.Sprintf("%dw ago", weeks)
	}
	if days := int(duration.Hours() / 24); days > 0 {
		return fmt.Sprintf("%dd ago", days)
	}
	if hours := int(duration.Hours()); hours > 0 {
		return fmt.Sprintf("%dh ago", hours)
	}
	if minutes := int(duration.Minutes()); minutes > 0 {
		return fmt.Sprintf("%dm ago", minutes)
	}
	return fmt.Sprintf("%ds ago", int(duration.Seconds()))
}

func IsAge(s string) bool {
	_, ok := ParseAge(s)
	return ok
}

// ParseAge parses an age string from Age() back into a duration (older = larger duration).
// Supports "0s", "Ns ago", "Nm ago", "Nh ago", "Nd ago", "Nw ago", "Nmo ago", "Ny ago".
// Returns (0, false) for non-age strings.
func ParseAge(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// "0s" (no "ago")
	if s == "0s" {
		return 0, true
	}
	parts := strings.Split(s, " ")
	if len(parts) != 2 || parts[1] != "ago" {
		return 0, false
	}
	// "Nunit" e.g. "5m", "2h", "1y"
	numUnit := parts[0]
	if len(numUnit) < 2 {
		return 0, false
	}
	// "mo" (months) before "m" (minutes) - must check before single-char parse
	if strings.HasSuffix(numUnit, "mo") {
		n, err := strconv.Atoi(numUnit[:len(numUnit)-2])
		if err != nil {
			return 0, false
		}
		return time.Duration(n) * 24 * 30 * time.Hour, true
	}
	n, err := strconv.Atoi(numUnit[:len(numUnit)-1])
	if err != nil {
		return 0, false
	}
	switch numUnit[len(numUnit)-1] {
	case 's':
		return time.Duration(n) * time.Second, true
	case 'm':
		return time.Duration(n) * time.Minute, true
	case 'h':
		return time.Duration(n) * time.Hour, true
	case 'd':
		return time.Duration(n) * 24 * time.Hour, true
	case 'w':
		return time.Duration(n) * 24 * 7 * time.Hour, true
	case 'y':
		return time.Duration(n) * 24 * 365 * time.Hour, true
	default:
		return 0, false
	}
}

// Return docker image registry and image name with tag
func ImageInfo(imageURL *string) (string, string) {
	if imageURL == nil {
		return EmptyText, EmptyText
	}
	url := *imageURL
	// Map of known registry domains to their short names
	registryMap := map[string]string{
		"docker.io":           "Docker Hub",
		"ecr.aws":             "Amazon ECR Public",
		".amazonaws.com":      "Amazon ECR",
		"gcr.io":              "Google GCR",
		"azurecr.io":          "Azure ACR",
		"registry.gitlab.com": "GitLab",
		"ghcr.io":             "GitHub",
		"quay.io":             "Quay",
	}

	// Default registry short name
	defaultRegistry := "Docker Hub"

	// Extract the domain and path from the image URL
	var domain, path string
	if strings.Contains(url, "/") {
		parts := strings.SplitN(url, "/", 2)
		domain = parts[0]
		path = parts[1]
	} else {
		// If there's no '/', it's an official image on Docker Hub
		path = url
		domain = "docker.io"
	}

	// Check for known registries by domain
	registryShortName := defaultRegistry
	for key, shortName := range registryMap {
		if strings.Contains(domain, key) {
			registryShortName = shortName
			break
		}
	}

	if strings.Contains(path, ":") {
		parts := strings.SplitN(path, ":", 2)
		imageName := parts[0]
		tag := parts[1]
		if len(tag) > 8 {
			tag = tag[:8] + "..."
		}
		path = imageName + ":" + tag
	}

	// Return the registry short name and the image name with tag
	return registryShortName, path
}

// Get service name by describe task group name
func GetServiceByTaskGroup(group *string) string {
	if group == nil {
		return EmptyText
	}

	if !strings.HasPrefix(*group, "service") {
		return EmptyText
	}

	parts := strings.Split(*group, ":")
	return parts[1]
}

// Duration calculates the time difference between two timestamps and returns it in a human-readable format
func Duration(start, end time.Time) string {
	duration := end.Sub(start)

	if duration < 0 {
		return EmptyText
	}

	if hours := int(duration.Hours()); hours > 24 {
		days := hours / 24
		return fmt.Sprintf("%dd", days)
	}
	if hours := int(duration.Hours()); hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	if minutes := int(duration.Minutes()); minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%ds", int(duration.Seconds()))
}
