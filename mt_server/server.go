package mt_server

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	log "github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	gnmipb "github.com/openconfig/gnmi/proto/gnmi"

	sdc "github.com/batmancn/sonic-telemetry/mt_data_client"
	spb "github.com/batmancn/sonic-telemetry/mt_proto"
)

var (
	supportedEncodings = []gnmipb.Encoding{gnmipb.Encoding_JSON, gnmipb.Encoding_JSON_IETF, gnmipb.Encoding_PROTO}
)

// Server manages a single gNMI Server implementation. Each client that connects
// via Subscribe or Get will receive a stream of updates based on the requested
// path. Set request is processed by server too.
type Server struct {
	s       *grpc.Server
	lis     net.Listener
	config  *Config
	cMu     sync.Mutex
	clients map[string]*Client
}

// Config is a collection of values for Server
type Config struct {
	// Port for the Server to listen on. If 0 or unset the Server will pick a port
	// for this Server.
	Port int64
}

func printTypedVal(val *gnmipb.TypedValue) {
	log.Infof("printTypedVal: %s", val.String())
}

// getFullPath builds the full path from the prefix and path.
func getFullPath(path *gnmipb.Path) string {
	fullPath := ""

	for _, pathElem := range(path.GetElem()) {
		fullPath += pathElem.GetName() + "/"
	}

    return fullPath
}

// New returns an initialized Server.
func NewServer(config *Config, opts []grpc.ServerOption) (*Server, error) {
	if config == nil {
		return nil, errors.New("config not provided")
	}

	s := grpc.NewServer(opts...)
	reflection.Register(s)

	srv := &Server{
		s:       s,
		config:  config,
		clients: map[string]*Client{},
	}
	var err error
	if srv.config.Port < 0 {
		srv.config.Port = 0
	}
	srv.lis, err = net.Listen("tcp", fmt.Sprintf(":%d", srv.config.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to open listener port %d: %v", srv.config.Port, err)
	}
	gnmipb.RegisterGNMIServer(srv.s, srv)
	log.V(1).Infof("Created Server on %s", srv.Address())
	return srv, nil
}

// Serve will start the Server serving and block until closed.
func (srv *Server) Serve() error {
	s := srv.s
	if s == nil {
		return fmt.Errorf("Serve() failed: not initialized")
	}
	return srv.s.Serve(srv.lis)
}

// Address returns the port the Server is listening to.
func (srv *Server) Address() string {
	addr := srv.lis.Addr().String()
	return strings.Replace(addr, "[::]", "localhost", 1)
}

// Port returns the port the Server is listening to.
func (srv *Server) Port() int64 {
	return srv.config.Port
}

// Subscribe implements the gNMI Subscribe RPC.
func (srv *Server) Subscribe(stream gnmipb.GNMI_SubscribeServer) error {
	ctx := stream.Context()

	pr, ok := peer.FromContext(ctx)
	if !ok {
		return grpc.Errorf(codes.InvalidArgument, "failed to get peer from ctx")
		//return fmt.Errorf("failed to get peer from ctx")
	}
	if pr.Addr == net.Addr(nil) {
		return grpc.Errorf(codes.InvalidArgument, "failed to get peer address")
	}

	/* TODO: authorize the user
	msg, ok := credentials.AuthorizeUser(ctx)
	if !ok {
		log.Infof("denied a Set request: %v", msg)
		return nil, status.Error(codes.PermissionDenied, msg)
	}
	*/

	c := NewClient(pr.Addr)

	srv.cMu.Lock()
	if oc, ok := srv.clients[c.String()]; ok {
		log.V(2).Infof("Delete duplicate client %s", oc)
		oc.Close()
		delete(srv.clients, c.String())
	}
	srv.clients[c.String()] = c
	srv.cMu.Unlock()

	err := c.Run(stream)
	srv.cMu.Lock()
	delete(srv.clients, c.String())
	srv.cMu.Unlock()

	log.Flush()
	return err
}

// checkEncodingAndModel checks whether encoding and models are supported by the server. Return error if anything is unsupported.
func (s *Server) checkEncodingAndModel(encoding gnmipb.Encoding, models []*gnmipb.ModelData) error {
	hasSupportedEncoding := false
	for _, supportedEncoding := range supportedEncodings {
		if encoding == supportedEncoding {
			hasSupportedEncoding = true
			break
		}
	}
	if !hasSupportedEncoding {
		return fmt.Errorf("unsupported encoding: %s", gnmipb.Encoding_name[int32(encoding)])
	}

	return nil
}

// Get implements the Get RPC in gNMI spec.
func (s *Server) Get(ctx context.Context, req *gnmipb.GetRequest) (*gnmipb.GetResponse, error) {
	var err error

	// check request type
	if req.GetType() != gnmipb.GetRequest_ALL {
		return nil, status.Errorf(codes.Unimplemented, "unsupported request type: %s", gnmipb.GetRequest_DataType_name[int32(req.GetType())])
	}

	if err = s.checkEncodingAndModel(req.GetEncoding(), req.GetUseModels()); err != nil {
		return nil, status.Error(codes.Unimplemented, err.Error())
	}

	// check target
	var target string
	prefix := req.GetPrefix()
	if prefix == nil {
		return nil, status.Error(codes.Unimplemented, "No target specified in prefix")
	} else {
		target = prefix.GetTarget()
		if target == "" {
			return nil, status.Error(codes.Unimplemented, "Empty target data not supported yet")
		}
	}

	var spbValues []*spb.Value
	paths := req.GetPath()
	log.Infof("target: " + target)
	if target == "MTNOS" {
		yc, err := sdc.NewYangClient(paths)

		if err != nil {
			return nil, status.Error(codes.NotFound, err.Error())
		}

		spbValues, err = yc.GetPb()
		if err != nil {
			return nil, status.Error(codes.NotFound, err.Error())
		}
	} else if target == "CONFIG_DB" || target == "COUNTERS_DB" {
		dc, err := sdc.NewDbClient(paths, prefix)

		if err != nil {
			return nil, status.Error(codes.NotFound, err.Error())
		}

		spbValues, err = dc.Get(nil)
		if err != nil {
			return nil, status.Error(codes.NotFound, err.Error())
		}
	} else {
		log.Infof("target: " + target)
		return nil, fmt.Errorf("target %s not supported", target)
	}

	// make getResponse
	notifications := make([]*gnmipb.Notification, len(paths))
	for index, spbValue := range spbValues {
		//printTypedVal(spbValue.GetVal())

		update := &gnmipb.Update{
			Path: spbValue.GetPath(),
			Val:  spbValue.GetVal(),
		}

		notifications[index] = &gnmipb.Notification{
			Timestamp: spbValue.GetTimestamp(),
			Prefix:    prefix,
			Update:    []*gnmipb.Update{update},
		}
		index++
	}
	return &gnmipb.GetResponse{Notification: notifications}, nil
}

// Set method
func (srv *Server) Set(ctx context.Context, req *gnmipb.SetRequest) (*gnmipb.SetResponse, error) {
	var target string

	// check setRequest type
	prefix := req.GetPrefix()
	if prefix == nil {
		return nil, status.Error(codes.Unimplemented, "No target specified in prefix")
	}

	// check target
	target = prefix.GetTarget()
	if target == "" {
		return nil, status.Error(codes.Unimplemented, "Empty target data not supported yet")
	} else if target != "MTNOS" {
		return nil, status.Errorf(codes.Unimplemented, "unsupported request target")
	}

	// new yc to process setRequest
	log.Infof("SetRequest: %v", req)
	var results []*gnmipb.UpdateResult
	yc, err := sdc.NewYangClient([]*gnmipb.Path{})
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	for _, path := range req.GetReplace() {
		log.Infof("Replace path: %s", getFullPath(path.GetPath()))

		err = yc.SetPb(path.GetPath(), path.GetVal())
		if err != nil {
			return nil, err
		}

		res := gnmipb.UpdateResult{
			Path: path.GetPath(),
			Op:   gnmipb.UpdateResult_REPLACE,
		}

		results = append(results, &res)
	}

	// make setResponse
	return &gnmipb.SetResponse{
		Timestamp: time.Now().UnixNano(),
		Prefix:    req.GetPrefix(),
		Response:  results,
	}, nil
}

// Capabilities method is not implemented. Refer to gnxi for examples with openconfig integration
func (srv *Server) Capabilities(context.Context, *gnmipb.CapabilityRequest) (*gnmipb.CapabilityResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "Capabilities() is not implemented")
}
