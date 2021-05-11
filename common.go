package main

func contain(listWorkerIDs []int, workerID int) bool {
	for _, id := range listWorkerIDs {
		if id == workerID {
			return true
		}
	}
	return false
}
