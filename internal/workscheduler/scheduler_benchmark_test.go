package workscheduler

import (
	"context"
	"testing"
)

type benchmarkResult struct{}
func (*benchmarkResult) Close() {}

type benchmarkJob struct{ result *benchmarkResult }
func (j *benchmarkJob) Run(context.Context) *benchmarkResult { return j.result }
func (*benchmarkJob) Close() {}

func BenchmarkSchedulerSubmitComplete(b *testing.B) {
	scheduler, err := New[uint64, uint64, *benchmarkResult](Options{Workers: 2, QueueCapacity: 32})
	if err != nil { b.Fatal(err) }
	defer scheduler.Close()
	job := &benchmarkJob{result: &benchmarkResult{}}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		key := uint64(index + 1)
		if err = scheduler.Submit(Work[uint64, uint64, *benchmarkResult]{Key: key, Owner: key, Job: job}); err != nil { b.Fatal(err) }
		<-scheduler.Ready()
		completion, ok := scheduler.TakeCompletion()
		if !ok || completion.Key != key { b.Fatal("completion mismatch") }
		scheduler.Finish(key)
	}
}
