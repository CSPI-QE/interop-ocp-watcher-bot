package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/tidwall/gjson"
	"google.golang.org/api/option"
)

// Initialize variables and flags
var job_file_path string
var mentioned_group_id string
var webhook_url string
var job_group_name string
var jobs []Job

var jobs_failed int
var jobs_succeeded int
var jobs_inactive int
var jobs_running int
var total_jobs_ran int

// Job struct
type Job struct {
	// Define struct variables
	Job_Name     string
	Active       bool
	Latest_Build string
	Successful   bool
	Running      bool
}

func init() {
	// Define flags and parse them
	flag.StringVar(&job_file_path, "job_file_path", "", "Path to JSON file holding list of active jobs.")
	flag.StringVar(&mentioned_group_id, "mentioned_group_id", "", "The user group ID of the group to mention in the message.")
	flag.StringVar(&webhook_url, "webhook_url", "", "The Slack webhook URL to use when sending a message.")
	flag.StringVar(&job_group_name, "job_group_name", "", "The name of the group of jobs to use in the message.")

	flag.Parse()

	if job_file_path == "" || webhook_url == "" || mentioned_group_id == "" || job_group_name == "" {
		slog.Error("Missing arguments (job_file_path, webhook_url, mentioned_group_id, or job_group_name). Exiting...")
		os.Exit(1)
	}
}

func main() {

	load_jobs(job_file_path)

	send_message(webhook_url, build_message(job_group_name, mentioned_group_id))
}

// load_jobs loads a JSON list of job names/active status.
// The function will populate the "jobs" array.
func load_jobs(job_file_path string) {
	// Open JSON file
	jsonFile, err := os.Open(job_file_path)
	if err != nil {
		slog.Error("Unable to open " + job_file_path)
		fmt.Println(err)
		os.Exit(1)
	}
	slog.Info("Successfully opened " + job_file_path)

	defer jsonFile.Close()

	// Unmarshall JSON
	Data, _ := io.ReadAll(jsonFile)
	err = json.Unmarshal(Data, &jobs)
	if err != nil {

		// if error is not nil
		// print error
		slog.Error("Error encountered while unmarshalling JSON")
		fmt.Println(err)
		os.Exit(1)
	}
	jsonFile.Close()

	slog.Info("Finding latest build IDs of jobs")
	for i := range jobs {
		// Populate the job struct with the latest build ID
		get_latest_build_id(&jobs[i])
	}

	slog.Info("Retrieving build statuses")
	for i := range jobs {
		// Populate the job struct with the status
		get_job_status(&jobs[i])
	}

	// Get counts of successful, failed, and inactive jobs
	for i := range jobs {
		if jobs[i].Successful && jobs[i].Active {
			jobs_succeeded++
		} else if !jobs[i].Successful && jobs[i].Active {
			jobs_failed++
		} else {
			jobs_inactive++
		}

		if jobs[i].Running {
			jobs_running++
		}
	}
}

// get_latest_build_id uses read_gcp_file to read the latest-build.txt file from the job's gcp folder
func get_latest_build_id(job *Job) {

	latest_build_id_file := "logs/" + string(job.Job_Name) + "/latest-build.txt"
	build_id := read_gcp_file("test-platform-results", latest_build_id_file)
	job.Latest_Build = string(build_id)
}

// get_job_status uses read_gcp_file to read the finished.json file for the build.
// It then parses the JSON to see if the build was a success.
func get_job_status(job *Job) {
	// Get contents of finished.json for build
	finished_file_path := "logs/" + string(job.Job_Name) + "/" + string(job.Latest_Build) + "/finished.json"
	finished_content := read_gcp_file("test-platform-results", finished_file_path)

	// Get the status and populate the job.Status
	status := gjson.Get(finished_content, "result")
	if status.String() == "SUCCESS" {
		job.Successful = true
	} else if status.String() == "FAILURE" {
		job.Successful = false
	} else {
		job.Running = true
	}
}

// read_gcp_file takes gcp_bucket and file as arguments and uses them to read the contents of the file.
// returns a string value which is the contents of the file
func read_gcp_file(gcp_bucket string, file string) string {
	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		slog.Error("Unable to create new storage client.")
		os.Exit(1)
	}

	rc, err := client.Bucket(gcp_bucket).Object(file).NewReader(ctx)
	if err != nil {
		slog.Info("Unable to open new reader for GCP Bucket. File " + file + " doesn't seem to exist.")
		return ""
	}
	slurp, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		slog.Error("Unable to read file in GCP bucket.")
		os.Exit(1)
	}
	return string(slurp)
}

// build_message builds the message the bot is going to send and returns a string.
func build_message(job_group_name string, mentioned_group_id string) string {
	// Greeting
	greeting := "Hello <!subteam^" + mentioned_group_id + "> :wave:, here is the weekly run report for " + job_group_name + ":\n\n"

	// Statistics block
	total_jobs_ran = jobs_succeeded + jobs_failed
	passing_percentage := math.Ceil((float64(jobs_succeeded) / float64(total_jobs_ran)) * 100)
	statistics_block := fmt.Sprintf("*Total:* %d\n*Successful:* %d\n*Failed:* %d\n*Inactive:* %d\n*Passing Percentage:* approximately %d%%\n", total_jobs_ran, jobs_succeeded, jobs_failed, jobs_inactive, int(passing_percentage))
	if jobs_running > 0 {
		statistics_block = statistics_block + fmt.Sprintf("*Jobs Still Running:* %d\n", jobs_running)
	}

	// Failed jobs list
	failed_jobs_title := "\n\n*FAILED JOBS*\n"
	var failed_jobs_list bytes.Buffer
	for i := range jobs {
		if !jobs[i].Successful && jobs[i].Active {
			list_item := "- <https://prow.ci.openshift.org/view/gs/test-platform-results/logs/" + jobs[i].Job_Name + "/" + jobs[i].Latest_Build + "|" + jobs[i].Job_Name + ">\n"
			failed_jobs_list.WriteString(list_item)
		}
	}
	failed_jobs_block := failed_jobs_title + failed_jobs_list.String() + "\n\n"

	// Inactive jobs list
	inactive_jobs_title := "*INACTIVE JOBS*\n"
	var inactive_jobs_list bytes.Buffer
	for i := range jobs {
		if !jobs[i].Active {
			list_item := "- " + jobs[i].Job_Name + "\n"
			inactive_jobs_list.WriteString(list_item)
		}
	}
	inactive_jobs_block := inactive_jobs_title + inactive_jobs_list.String() + "\n"

	// Running jobs list
	if jobs_running > 0 {
		jobs_running_title := "\n*JOBS RUNNING*\n"
		var jobs_running_list bytes.Buffer
		for i := range jobs {
			if jobs[i].Running {
				list_item := "- <https://prow.ci.openshift.org/view/gs/test-platform-results/logs/" + jobs[i].Job_Name + "/" + jobs[i].Latest_Build + "|" + jobs[i].Job_Name + ">\n"
				jobs_running_list.WriteString(list_item)
			}
		}
		jobs_running_block := jobs_running_title + jobs_running_list.String() + "\n"

		return greeting + statistics_block + failed_jobs_block + inactive_jobs_block + jobs_running_block
	}

	return greeting + statistics_block + failed_jobs_block + inactive_jobs_block
}

// Sends a message using the Slack webhook
func send_message(webhook_url string, message string) {
	slog.Info("Sending message")

	client := &http.Client{}
	var data = strings.NewReader("{\"text\":\"" + message + "\"}")
	req, err := http.NewRequest("POST", webhook_url, data)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	req.Header.Set("Content-type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	fmt.Printf("Message response: %s\n", bodyText)
}
