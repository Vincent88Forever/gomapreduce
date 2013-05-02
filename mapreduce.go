/*
Package mapreduce implements a MapReduce library.
*/

package gomapreduce

import (
	"net"          // interface for network I/O
	"net/rpc"
	"log"
	"os"
	"syscall"
	"sync"
	"fmt"
	"math/rand"
	//"math"
	"time"
	"encoding/gob"
)

type MapReduceNode struct {
	mu sync.Mutex         // singleton mutex for node
	l net.Listener        // RPC network listener
	dead bool             // used for testing dead nodes
	unreliable bool       // used for testing unreliable nodes
	rpcCount int          // maintain count of RPC calls

	me int                // index into nodes
	nodes []string        // MapReduceNode port names
	node_count int
	net_mode string       // "unix" or "tcp"

	// State for master role
	jobs map[string] Job   // Maps string job_id -> Job
  tm TaskManager        // all Tasks the MapReduceNode while acting as a master

	// State for worker roles
  intermediates map[MediateTuple][]Pair
  // Last ping times for each other node
  lastPingTimes map[string]time.Time
  // State ("idle", "dead", etc.) of each other node
  nodeStates map[string]string
}


// Master stores state about each running task
type TaskState struct {
	Key string      // The key where the data for the job can be found
	Worker string   // The node assigned as the mapper for this job. Empty string if no assigned worker
	Completed bool  // Whether the job is completed or not
}


/*
Client would like to start a Job instance which is composed of Task 
instances (MapTasks or Reduce Tasks). Client passes his implemented Mapper,
Reducer, and JobConfig instance. Any configuration settings not for a 
particular job should be read from the environment.
Spawns a master_role thread to perform the requested Job by breaking it into 
tasks that are allocated to workers.

Aside: Currently, this is called from a client which has a local MapReduceNode running
at it, but a wrapper that allows start to be called remotely via RPC could be 
created. We don't currently have any scenarios where the client is not also a 
member of the network but it is totally possible.
*/
func (self *MapReduceNode) Start(mapper Mapper, reducer Reducer, job_config JobConfig) string {
	fmt.Println("Start MapReduce", mapper, reducer)
  self.broadcast_testrpc(mapper)          // temporary

  job_id := generate_uuid()       // Job identifier created internally, unlike in Paxos
  job := Job{job_id: job_id, 
             finished: false, 
             master: self.me,  // storing index into nodes prevents long server names being shown upon printing job 
             status: "starting",
             mapper: mapper,
             reducer: reducer,
             inputAccessor: makeS3Accessor("small_test"),
            }
  self.jobs[job.get_id()] = job
  fmt.Println(job)

	// TODO should this node be the master, or should it pick a master at random? Using this node as the maste for now.
	// Good point, I suppose the question here how to handle a single client who wants to do a lot of jobs. 
	go self.master_role(job, job_config)  // TODO should be RPC call to master instead // I don't think it should be an RPC? 

  return job_id
}


/*
Performs the requested Job by breaking it into tasks based on the JobConfig,
allocating the Tasks to workers, and monitors progress on the Job until it is 
complete.
The method used by the master node to start the entire mapreduce operation
*/
func (self *MapReduceNode) master_role(job Job, config JobConfig) {
	debug(fmt.Sprintf("(svr:%d) master_role: job", self.me))

	// Split input data into M components. Note this is done already as part of S3 code.

	// Construct the M MapTasks that must be performed for the Job.
  map_tasks := makeMapTasks(job, config)

	fmt.Printf("Tasks: %v\n", map_tasks)

	// Assign the map jobs to workers
	//self.assignMapJobs(mapJobs)
}


// Exported RPC functions (internal to mapreduce service)
///////////////////////////////////////////////////////////////////////////////

// Accepts a request to perform a MapTask or an AcceptTask. May decline the
// request if overworked
func (self *MapReduceNode) ReceiveTask(args *AssignTaskArgs, reply *AssignTaskReply) error {
	debug(fmt.Sprintf("Received a task"))

	//fmt.Printf("Worker %d starting Map(%s)\n", self.me, args.Job.Key)
	//mapData, _ := self.bucket.GetObject(args.Job.Key)
	//fmt.Printf("Worker %d got map data: %s\n", self.me, string(mapData[:int(math.Min(30, float64(len(mapData))))]))
	// TODO Run the map function on the data
	// TODO Write the intermediate keys/values to somewhere (in memory for now) so it can be fetched by reducers later

	reply.OK = true

	return nil

}

//func (self *MapReduceNode) Get(...)


// A method used by a map worker. The worker will fetch the data associated with the key for the job it's assigned, and
// then run the map function on that data. The worker stores the intermediate key/value pairs in memory and tells the
// master where those values are stored so that reduce workers can get them when needed.
func (self *MapReduceNode) StartMapJob(args *AssignTaskArgs, reply *AssignTaskReply) error{
	//fmt.Printf("Worker %d starting Map(%s)\n", self.me, args.Job.Key)
	//mapData, _ := self.bucket.GetObject(args.Job.Key)
	//fmt.Printf("Worker %d got map data: %s\n", self.me, string(mapData[:int(math.Min(30, float64(len(mapData))))]))
	// TODO Run the map function on the data
	// TODO Write the intermediate keys/values to somewhere (in memory for now) so it can be fetched by reducers later

	reply.OK = true

	return nil
}

//TaskState{key, "", false}
// Helpers
///////////////////////////////////////////////////////////////////////////////

// Gets all the keys that need to be mapped via MapTasks for the job and 
// constructs MapTask instances. Returns the list of MapTasks
func makeMapTasks(job Job, config JobConfig) []MapTask {
  var task_list []MapTask

  // Assumes the Job input is prechunked
	for _, key := range job.inputAccessor.listKeys() {
    task_id := generate_uuid()
    maptask := makeMapTask(task_id, key, job.mapper)
    task_list = append(task_list, maptask)   // Construct a new job object and append to the list of them
	}
	return task_list
}

// Method used by the master. Assigns jobs to workers until all jobs are complete.
func (self *MapReduceNode) assignMapJobs(jobs []TaskState) {
	// workers := append(self.nodes[:self.me], self.nodes[self.me:]...)  // The workers available for map tasks (everyone but me)
	// fmt.Printf("Map Workers: %s\n", workers)

	// numUnfinished := getNumberUnfinished(jobs)    // The number of jobs that are not complete
	// //var job TaskState

	// for numUnfinished > 0 {   // While there are jobs left to complete
	// 	fmt.Printf("Number unfinished: %d\n", numUnfinished)
	// 	jobIndex := getUnassignedJob(jobs)  // Get the index of one of the unassigned jobs

	// 	if jobIndex != -1 {     // A value of -1 means that all jobs are assigned
	// 		job = jobs[jobIndex]
	// 		worker := workers[rand.Intn(len(workers))]    // Get a random worker. TODO should only use an idle node
	// 		jobs[jobIndex].Worker = worker  // Assign the worker for the job
	// 		//args := AssignTaskArgs{job}
	// 		//reply := &AssignTaskReply{}

	// 		//self.call(worker, "MapReduce.StartMapJob", args, reply)   // TODO this should be asynchronous RPC, check for err etc.
	// 		jobs[jobIndex].Completed = true     // TODO job should only be set to complete when the worker actually finishes it
	// 	}

	// 	numUnfinished = getNumberUnfinished(jobs)
		// TODO should sleep for some amount of time before looping again?
	// }
}

// Iterates though jobs and counts the number that are unfinished.
func getNumberUnfinished(jobs []TaskState) int{
	unfinished := 0
	for _, job := range jobs {
		if !job.Completed {
			unfinished++
		}
	}

	return unfinished
}

// Gets a random unassigned job from the list of jobs and returns it
func getUnassignedJob(jobs []TaskState) int {
	for i, job := range jobs {
		if job.Worker == "" {
			return i
		}
	}

	return -1  // TODO check this
}


func (self *MapReduceNode) tick() {
	// fmt.Println("Tick")
	self.sendPings()
	self.checkForDisconnectedNodes()
}


func (self *MapReduceNode) broadcast_testrpc(maptask Mapper) {
  fmt.Println(self.nodes, maptask)
  for index, node := range self.nodes {
    if index == self.me {
      continue
    }
    args := &TestRPCArgs{}         // declare and init zero valued struct
    args.Number = rand.Int()
    //task := ExampleMapper{}
    args.Mapper = maptask
    var reply TestRPCReply
    ok := self.call(node, "MapReduceNode.TestRPC", args, &reply)
    if ok {
      fmt.Println("Successfully sent")
      fmt.Println(reply)
    } else {
      fmt.Println("Sent but not received")
      fmt.Println(reply)
    }
  }
  return
}

// Helper method for a node to send a ping to all of its peers
func (self *MapReduceNode) sendPings() {
	args := &PingArgs{self.nodes[self.me]}
	for _, node := range self.nodes {
		if node != self.nodes[self.me]{
			reply := &PingReply{}
			self.call(node, "MapReduceNode.HandlePing", args, reply)
		}
	}
}

// Ping handler, called via RPC when a node wants to ping this node
func (self *MapReduceNode) HandlePing(args *PingArgs, reply *PingReply) error {
	fmt.Printf("Node %d receiving ping from Node %s\n", self.me, args.Me[len(args.Me)-1:])

	self.lastPingTimes[args.Me] = time.Now()

	return nil
}

// checks to see when each of the other nodes last pinged. If it was too long ago, mark them as dead
func (self *MapReduceNode) checkForDisconnectedNodes() {
	for node, lastPingTime := range self.lastPingTimes {
		timeDifference := time.Now().Sub(lastPingTime)
		if timeDifference > DeadPings * PingInterval {
			fmt.Printf("Node %d marking node %s as dead\n", self.me, node)
			self.nodeStates[node] = "dead"
		}
	}
}


// Handle test RPC RPC calls.
func (self *MapReduceNode) TestRPC(args *TestRPCArgs, reply *TestRPCReply) error {
  fmt.Println("Received TestRPC", args.Number)
  result := args.Mapper.Map("This is a sample string sample string is is")       // perform work on a random input
  fmt.Println(result)
  //fmt.Printf("Task id: %d\n", args.Mapper.get_id())
  reply.Err = OK
  return nil
}


//
// call() sends an RPC to the rpcname handler on server srv
// with arguments args, waits for the reply, and leaves the
// reply in reply. the reply argument should be a pointer
// to a reply structure.
//
// the return value is true if the server responded, and false
// if call() was not able to contact the server. in particular,
// the reply's contents are only valid if call() returned true.
//
// you should assume that call() will time out and return an
// error after a while if it doesn't get a reply from the server.
//
// please use call() to send all RPCs, in client.go and server.go.
// please don't change this function.
//
func (self *MapReduceNode) call(srv string, rpcname string, args interface{}, reply interface{}) bool {
	fmt.Println("Sending to", srv)
	c, errx := rpc.Dial("unix", srv)
	if errx != nil {
		fmt.Printf("Error: %s\n", errx)
		return false
	}
	defer c.Close()
		
	err := c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}
	fmt.Printf("Error: %s\n", err)
	return false
}  


//
// tell the peer to shut itself down.
// for testing.
// please do not change this function.
//
func (self *MapReduceNode) Kill() {
	self.dead = true
	if self.l != nil {
		self.l.Close()
	}
}


// Create an MapReduceNode instance
// The ports of all the nodes (including this one) are in nodes[], 
// this node's port is nodes[me]

func Make(nodes []string, me int, rpcs *rpc.Server, mode string) *MapReduceNode {
  // Initialize a MapReduceNode
  mr := &MapReduceNode{}
  mr.nodes = nodes    
  mr.me = me
  mr.net_mode = mode
  // Initialization code
  mr.node_count = len(nodes)
  mr.jobs = make(map[string]Job)
  mr.tm = makeTaskManager()
  // Initialize last ping times for each node
  mr.nodeStates = map[string]string{}
  mr.lastPingTimes = map[string]time.Time{}
  for _, node := range mr.nodes {
  	if node == mr.nodes[mr.me] { 	// Skip ourself
  		continue
  	}
  	mr.lastPingTimes[node] = time.Now()
  	mr.nodeStates[node] = "idle"
  }

  if rpcs != nil {
    rpcs.Register(mr)      // caller created RPC Server
  } else {
    rpcs = rpc.NewServer() // creates a new RPC server
    rpcs.Register(mr)      // Register exported methods of MapReduceNode with RPC Server         
    gob.Register(TestRPCArgs{})
    gob.Register(TestRPCReply{})

    // Prepare node to receive connections

    if mode == "tcp" {
      fmt.Println("Making in TCP mode")
      listener, error := net.Listen("tcp", ":8080");

      if error != nil {
        log.Fatal("listen error: ", error);
      }
      mr.l = listener      // Set MapReduceNode listener
      go func() {
        for mr.dead == false {
          conn, err := mr.l.Accept()
          if err == nil && mr.dead == false {
            if mr.unreliable && (rand.Int63() % 1000) < 100 {
              // discard the request.
              conn.Close()
            } else if mr.unreliable && (rand.Int63() % 1000) < 200 {
              // process the request but force discard of reply.
              c1 := conn.(*net.UnixConn)
              f, _ := c1.File()
              err := syscall.Shutdown(int(f.Fd()), syscall.SHUT_WR)
              if err != nil {
                fmt.Printf("shutdown: %v\n", err)
              }
              mr.rpcCount++
              go rpcs.ServeConn(conn)
            } else {
              mr.rpcCount++
              go rpcs.ServeConn(conn)
            }
          } else if err == nil {
            conn.Close()
          }
          if err != nil && mr.dead == false {
            fmt.Printf("Paxos(%v) accept: %v\n", me, err.Error())
          }
        }
      }()

    } else {
      fmt.Println("Making in Unix mode")
      // mode assumed to be "unix"

      os.Remove(nodes[me]) // only needed for "unix"
      listener, error := net.Listen("unix", nodes[me]);
      if error != nil {
        log.Fatal("listen error: ", error);
      }
      mr.l = listener      // Set MapReduceNode listener

      

      // please do not change any of the following code,
      // or do anything to subvert it.
      
      // create a thread to accept RPC connections
      go func() {
        for mr.dead == false {
          conn, err := mr.l.Accept()
          if err == nil && mr.dead == false {
            if mr.unreliable && (rand.Int63() % 1000) < 100 {
              // discard the request.
              conn.Close()
            } else if mr.unreliable && (rand.Int63() % 1000) < 200 {
              // process the request but force discard of reply.
              c1 := conn.(*net.UnixConn)
              f, _ := c1.File()
              err := syscall.Shutdown(int(f.Fd()), syscall.SHUT_WR)
              if err != nil {
                fmt.Printf("shutdown: %v\n", err)
              }
              mr.rpcCount++
              go rpcs.ServeConn(conn)
            } else {
              mr.rpcCount++
              go rpcs.ServeConn(conn)
            }
          } else if err == nil {
            conn.Close()
          }
          if err != nil && mr.dead == false {
            fmt.Printf("Paxos(%v) accept: %v\n", me, err.Error())
          }
        }
      }()
    }

  }

  go func() {
    for mr.dead == false {
      mr.tick()
      time.Sleep(250 * time.Millisecond)
    }
  }()

  return mr
}


