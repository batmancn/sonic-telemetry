package client

import (
    "fmt"

    log "github.com/golang/glog"
    sonic_proto "github.com/Azure/sonic-telemetry/proto"
    gnmipb "github.com/openconfig/gnmi/proto/gnmi"
    proto "github.com/golang/protobuf/proto"
)

func checkAllowedTypedValue(val *gnmipb.TypedValue) error {
    switch val.Value.(type) {
    case *gnmipb.TypedValue_ProtoBytes:
        return nil
    default:
        return fmt.Errorf("Only support TypedValue_ProtoBytes")
    }
}

func staticRoutesStaticFunc (val *gnmipb.TypedValue) error {
    err := checkAllowedTypedValue(val)
    if err != nil {
		return err
	}

    srs := &sonic_proto.StaticRoutesStatic{}
    err = proto.Unmarshal(val.GetProtoBytes(), srs)
	if err != nil {
		return err
	}

    op_str := ""
    addr := srs.GetAddress()
    next_hops := srs.GetNextHops().GetNextHop()

    op_str += addr
    op_str += " "

    for _, v := range(next_hops) {
        next_hop_str := v.GetConfig().GetNextHop()
        metric := v.GetConfig().GetMetric()

        if metric == 0 {
            op_str = op_str + next_hop_str + " "
        } else {
            op_str = op_str + next_hop_str + fmt.Sprint(metric) + " "
        }
    }

    log.Infof("Run CLI to set config: config static-route %v", op_str)
    // TBD, use CLI to add static route

    return nil
}

type yangDataGetFunc func(val *gnmipb.TypedValue) error

type path2DataReplaceFunc struct {
    path string
    replaceFunction yangDataGetFunc
}

var (
    path2DataReplaceFuncTbl = []path2DataReplaceFunc{
        {
            path: "local-routes/static-routes/static/",
            replaceFunction: staticRoutesStaticFunc,
        },
    }
)

type YangClient struct {
}

func (yc *YangClient) CheckYangPath (path *string) (int, error) {
    err := fmt.Errorf("Err: path not found")
    path_no := 0

    log.Infof("path : %v", *path)

    for i, v := range(path2DataReplaceFuncTbl) {
        log.Infof("path : %v", v.path)
        if *path == v.path {
            err = nil
            path_no = i
            break
        }
    }

    return path_no, err
}

func (yc *YangClient) SetPb (path *string, val *gnmipb.TypedValue) error {
	log.Infof("Set: path %v", path)

	path_no, err := yc.CheckYangPath(path)
	if err != nil {
		return err
    }

	return path2DataReplaceFuncTbl[path_no].replaceFunction(val)
}

func NewYangClient () (*YangClient, error) {
    var client = new(YangClient)
    return client, nil
}