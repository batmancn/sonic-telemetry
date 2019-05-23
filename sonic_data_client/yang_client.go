package client

type YangClient struct {
	prefix      *gnmipb.Path
	path2Getter map[*gnmipb.Path]dataGetFunc

	q       *queue.PriorityQueue
	channel chan struct{}

	synced sync.WaitGroup  // Control when to send gNMI sync_response
	w      *sync.WaitGroup // wait for all sub go routines to finish
	mu     sync.RWMutex    // Mutex for data protection among routines for DbClient

	sendMsg int64
	recvMsg int64
	errors  int64
}

func (yc YangClient) SetPb(path *gnmipb.Path, value value) error {
	// check path
	// unmashell value based on path
	// use
	//   1)template and value to load json
	//   2)CLI
	// CLI is better

	// 1. port up/down

	// 2. static route
}