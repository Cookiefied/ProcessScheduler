package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sort"
	"math"

	"github.com/olekukonko/tablewriter"
)

func main() {
	// CLI args
	f, closeFile, err := openProcessingFile(os.Args...)
	if err != nil {
		log.Fatal(err)
	}
	defer closeFile()

	// Load and parse processes
	processes, err := loadProcesses(f)
	if err != nil {
		log.Fatal(err)
	}

	// First-come, first-serve scheduling
	FCFSSchedule(os.Stdout, "First-come, first-serve", processes)

	// Shortest Job First Preemptive Scheduling
	SJFSchedule(os.Stdout, "Shortest-job-first", processes)
	
	// Shortest Job First Preemptive, Priority Scheduling
	SJFPrioritySchedule(os.Stdout, "Priority", processes)
	
	// Round-Robin Preemptive Scheduling
	// By default, we use a time quantum of 1 second. This can be changed below by changing 1 to a different value
	RRSchedule(os.Stdout, "Round-robin", processes, 1)
}

func openProcessingFile(args ...string) (*os.File, func(), error) {
	if len(args) != 2 {
		return nil, nil, fmt.Errorf("%w: must give a scheduling file to process", ErrInvalidArgs)
	}
	// Read in CSV process CSV file
	f, err := os.Open(args[1])
	if err != nil {
		return nil, nil, fmt.Errorf("%v: error opening scheduling file", err)
	}
	closeFn := func() {
		if err := f.Close(); err != nil {
			log.Fatalf("%v: error closing scheduling file", err)
		}
	}

	return f, closeFn, nil
}

type (
	Process struct {
		ProcessID     int64
		ArrivalTime   int64
		BurstDuration int64
		Priority      int64
	}
	TimeSlice struct {
		PID   int64
		Start int64
		Stop  int64
	}
)

//region Schedulers

// FCFSSchedule outputs a schedule of processes in a GANTT chart and a table of timing given:
// • an output writer
// • a title for the chart
// • a slice of processes
func FCFSSchedule(w io.Writer, title string, processes []Process) {
	var (
		serviceTime     int64
		totalWait       float64
		totalTurnaround float64
		lastCompletion  float64
		waitingTime     int64
		schedule        = make([][]string, len(processes))
		gantt           = make([]TimeSlice, 0)
	)
	for i := range processes {
		if processes[i].ArrivalTime > 0 {
			waitingTime = serviceTime - processes[i].ArrivalTime
		}
		totalWait += float64(waitingTime)

		start := waitingTime + processes[i].ArrivalTime

		turnaround := processes[i].BurstDuration + waitingTime
		totalTurnaround += float64(turnaround)

		completion := processes[i].BurstDuration + processes[i].ArrivalTime + waitingTime
		lastCompletion = float64(completion)

		schedule[i] = []string{
			fmt.Sprint(processes[i].ProcessID),
			fmt.Sprint(processes[i].Priority),
			fmt.Sprint(processes[i].BurstDuration),
			fmt.Sprint(processes[i].ArrivalTime),
			fmt.Sprint(waitingTime),
			fmt.Sprint(turnaround),
			fmt.Sprint(completion),
		}
		serviceTime += processes[i].BurstDuration

		gantt = append(gantt, TimeSlice{
			PID:   processes[i].ProcessID,
			Start: start,
			Stop:  serviceTime,
		})
	}

	count := float64(len(processes))
	aveWait := totalWait / count
	aveTurnaround := totalTurnaround / count
	aveThroughput := count / lastCompletion

	outputTitle(w, title)
	outputGantt(w, gantt)
	outputSchedule(w, schedule, aveWait, aveTurnaround, aveThroughput)
}

func SJFSchedule(w io.Writer, title string, processes []Process) { 
	var (
		serviceTime     int64
		totalWait       float64
		totalTurnaround float64
		lastCompletion  float64
		waitingTime     int64
		schedule        = make([][]string, len(processes))
		gantt           = make([]TimeSlice, 0)
	)
	// Sort processes by burst duration in ascending order
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].BurstDuration < processes[j].BurstDuration
	})
	for i := range processes {
		if processes[i].ArrivalTime > 0 {
			waitingTime = serviceTime - processes[i].ArrivalTime
		}
		totalWait += float64(waitingTime)

		start := waitingTime + processes[i].ArrivalTime

		turnaround := processes[i].BurstDuration + waitingTime
		totalTurnaround += float64(turnaround)

		completion := processes[i].BurstDuration + processes[i].ArrivalTime + waitingTime
		lastCompletion = float64(completion)

		schedule[i] = []string{
			fmt.Sprint(processes[i].ProcessID),
			fmt.Sprint(processes[i].Priority),
			fmt.Sprint(processes[i].BurstDuration),
			fmt.Sprint(processes[i].ArrivalTime),
			fmt.Sprint(waitingTime),
			fmt.Sprint(turnaround),
			fmt.Sprint(completion),
		}
		serviceTime += processes[i].BurstDuration

		gantt = append(gantt, TimeSlice{
			PID:   processes[i].ProcessID,
			Start: start,
			Stop:  serviceTime,
		})
	}

	count := float64(len(processes))
	aveWait := totalWait / count
	aveTurnaround := totalTurnaround / count
	aveThroughput := count / lastCompletion

	outputTitle(w, title)
	outputGantt(w, gantt)
	outputSchedule(w, schedule, aveWait, aveTurnaround, aveThroughput)
}

func SJFPrioritySchedule(w io.Writer, title string, processes []Process) {
	var (
		serviceTime     int64
		totalWait       float64
		totalTurnaround float64
		lastCompletion  float64
		waitingTime     int64
		schedule        = make([][]string, len(processes))
		gantt           = make([]TimeSlice, 0)
	)

	// Sort processes by arrival time in ascending order
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].ArrivalTime < processes[j].ArrivalTime
	})

	// Keep track of the index of the last process that has been executed
	lastExecuted := 0

	// Execute processes in order of arrival time until all have been executed
	for len(processes) > lastExecuted {
		// Find the process with the shortest remaining burst duration
		nextProcess := -1
		for i := lastExecuted; i < len(processes); i++ {
			if processes[i].ArrivalTime <= serviceTime {
				if nextProcess == -1 || processes[i].BurstDuration < processes[nextProcess].BurstDuration {
					nextProcess = i
				}
			} else {
				break
			}
		}

		if nextProcess == -1 {
			// No process is available to execute, so skip ahead to the next arrival time
			serviceTime = processes[lastExecuted].ArrivalTime
		} else {
			// Execute the selected process
			p := processes[nextProcess]

			if p.ArrivalTime > 0 {
				waitingTime = serviceTime - p.ArrivalTime
			}
			totalWait += float64(waitingTime)

			start := waitingTime + p.ArrivalTime

			turnaround := p.BurstDuration + waitingTime
			totalTurnaround += float64(turnaround)

			completion := p.BurstDuration + p.ArrivalTime + waitingTime
			lastCompletion = float64(completion)

			schedule[nextProcess] = []string{
				fmt.Sprint(p.ProcessID),
				fmt.Sprint(p.Priority),
				fmt.Sprint(p.BurstDuration),
				fmt.Sprint(p.ArrivalTime),
				fmt.Sprint(waitingTime),
				fmt.Sprint(turnaround),
				fmt.Sprint(completion),
			}
			serviceTime += p.BurstDuration

			gantt = append(gantt, TimeSlice{
				PID:   p.ProcessID,
				Start: start,
				Stop:  serviceTime,
			})

			// Remove the executed process from the list of processes
			processes[nextProcess] = processes[lastExecuted]
			lastExecuted++
		}
	}

	count := float64(len(processes))
	aveWait := totalWait / count
	aveTurnaround := totalTurnaround / count
	aveThroughput := count / lastCompletion

	outputTitle(w, title)
	outputGantt(w, gantt)
	outputSchedule(w, schedule, aveWait, aveTurnaround, aveThroughput)
}

func RRSchedule(w io.Writer, title string, processes []Process, timeSlice float64) {
	var (
		currentTime     int64
		totalWait       float64
		totalTurnaround float64
		queue           = make([]Process, 0)
		schedule        = make([][]string, len(processes))
		gantt           = make([]TimeSlice, 0)
	)

	// Sort processes by arrival time
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].ArrivalTime < processes[j].ArrivalTime
	})

	// Run the scheduling algorithm
	for len(queue) > 0 || len(processes) > 0 {
		// Add any arriving processes to the queue
		for len(processes) > 0 && processes[0].ArrivalTime <= currentTime {
			queue = append(queue, processes[0])
			processes = processes[1:]
		}

		// If the queue is empty, jump to the next process arrival time
		if len(queue) == 0 {
			currentTime = processes[0].ArrivalTime
		}

		// Get the next process in the queue
		process := queue[0]
		queue = queue[1:]

		// Run the process for the time slice
		var (
			start       = currentTime
			completion  int64
			burstLeft   = process.BurstDuration
			timeElapsed int64
		)
		for burstLeft > 0 {
			// Use up the time slice or the remaining burst time, whichever is shorter
			timeSpent := math.Min(float64(burstLeft), timeSlice)

			// )Update the completion time and elapsed time
			completion = int64(float64(currentTime) + timeSpent)
			timeElapsed += int64(timeSpent)
			currentTime = completion
			burstLeft -= int64(timeSpent)

			// Add to the Gantt chart
			gantt = append(gantt, TimeSlice{
				PID:   process.ProcessID,
				Start: start,
				Stop:  completion,
			})

			// Add any arriving processes to the queue
			for len(processes) > 0 && processes[0].ArrivalTime <= currentTime {
				queue = append(queue, processes[0])
				processes = processes[1:]
			}
		}

		// Calculate waiting and turnaround time for the process
		waitingTime := currentTime - process.ArrivalTime - process.BurstDuration
		turnaround := waitingTime + process.BurstDuration + timeElapsed
		totalWait += float64(waitingTime)
		totalTurnaround += float64(turnaround)

		// Add the process to the schedule table
		schedule[process.ProcessID-1] = []string{
			fmt.Sprint(process.ProcessID),
			fmt.Sprint(process.Priority),
			fmt.Sprint(process.BurstDuration),
			fmt.Sprint(process.ArrivalTime),
			fmt.Sprint(waitingTime),
			fmt.Sprint(turnaround),
			fmt.Sprint(completion),
		}
	}

	// Calculate and output statistics
	count := float64(len(schedule))
	aveWait := totalWait / count
	aveTurnaround := totalTurnaround / count
	aveThroughput := count / float64(gantt[len(gantt)-1].Stop)

	outputTitle(w, title)
	outputGantt(w, gantt)
	outputSchedule(w, schedule, aveWait, aveTurnaround, aveThroughput)
}

//endregion

//region Output helpers

func outputTitle(w io.Writer, title string) {
	_, _ = fmt.Fprintln(w, strings.Repeat("-", len(title)*2))
	_, _ = fmt.Fprintln(w, strings.Repeat(" ", len(title)/2), title)
	_, _ = fmt.Fprintln(w, strings.Repeat("-", len(title)*2))
}

func outputGantt(w io.Writer, gantt []TimeSlice) {
	_, _ = fmt.Fprintln(w, "Gantt schedule")
	_, _ = fmt.Fprint(w, "|")
	for i := range gantt {
		pid := fmt.Sprint(gantt[i].PID)
		padding := strings.Repeat(" ", (8-len(pid))/2)
		_, _ = fmt.Fprint(w, padding, pid, padding, "|")
	}
	_, _ = fmt.Fprintln(w)
	for i := range gantt {
		_, _ = fmt.Fprint(w, fmt.Sprint(gantt[i].Start), "\t")
		if len(gantt)-1 == i {
			_, _ = fmt.Fprint(w, fmt.Sprint(gantt[i].Stop))
		}
	}
	_, _ = fmt.Fprintf(w, "\n\n")
}

func outputSchedule(w io.Writer, rows [][]string, wait, turnaround, throughput float64) {
	_, _ = fmt.Fprintln(w, "Schedule table")
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"ID", "Priority", "Burst", "Arrival", "Wait", "Turnaround", "Exit"})
	table.AppendBulk(rows)
	table.SetFooter([]string{"", "", "", "",
		fmt.Sprintf("Average\n%.2f", wait),
		fmt.Sprintf("Average\n%.2f", turnaround),
		fmt.Sprintf("Throughput\n%.2f/t", throughput)})
	table.Render()
}

//endregion

//region Loading processes.
var ErrInvalidArgs = errors.New("invalid args")

func loadProcesses(r io.Reader) ([]Process, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: reading CSV", err)
	}

	processes := make([]Process, len(rows))
	for i := range rows {
		processes[i].ProcessID = mustStrToInt(rows[i][0])
		processes[i].BurstDuration = mustStrToInt(rows[i][1])
		processes[i].ArrivalTime = mustStrToInt(rows[i][2])
		if len(rows[i]) == 4 {
			processes[i].Priority = mustStrToInt(rows[i][3])
		}
	}

	return processes, nil
}

func mustStrToInt(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return i
}

//endregion
