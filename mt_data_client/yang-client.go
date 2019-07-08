package client

import (
	"strings"
    "bytes"
    "fmt"
    "os/exec"
    "sync"
    "encoding/json"
    "regexp"

    log "github.com/golang/glog"
    gnmipb "github.com/openconfig/gnmi/proto/gnmi"
    proto "github.com/golang/protobuf/proto"
    "github.com/workiva/go-datastructures/queue"

    mt_proto "github.com/batmancn/sonic-telemetry/mt_proto"
)

type ConfigClient struct {
}

func NewConfigClient() (*ConfigClient, error) {
    return new(ConfigClient), nil
}

func (cc *ConfigClient) parseIntfAddr(jsonStr []byte, intfName string) (*mt_proto.AddressesAddress) {
    addr := new(mt_proto.AddressesAddress)

    var f interface{}
    err := json.Unmarshal(jsonStr, &f)

    // fmt.Println("parseIntfAddr: ")
    if err == nil {
        m := f.(map[string]interface{})
        for k, v := range m {
            // fmt.Println(k)
            // fmt.Println(v)

            rep := intfName + ":.*"
            match, _ := regexp.MatchString(rep, k)
            if match {
                subpos := strings.Index(k, ":")
                addr.Interface = k[0:subpos]
                addr.Addr = k[subpos+1:len(k)]

                vaddr := v.(map[string]interface{})
                addr.Scope = vaddr["scope"].(string)
                addr.Family = vaddr["family"].(string)

                break;
            }
        }

        return addr
    } else {
        fmt.Println(err.Error())
        return nil
    }
}

func (cc *ConfigClient) parseBgpNa(jsonStr []byte) (*mt_proto.NeighborAddresses) {
    nas := new(mt_proto.NeighborAddresses)

    var f interface{}
    err := json.Unmarshal(jsonStr, &f)

    //fmt.Println("parseBgpNa: ")
    if err == nil {
        m := f.(map[string]interface{})
        for k, v := range m {
            na := new(mt_proto.NeighborAddress)

            // fmt.Println(k)
            na.RemoteAddr = k

            // fmt.Println(v)
            vna := v.(map[string]interface{})
            na.LocalAddr = vna["local_addr"].(string)
            na.Asn = vna["asn"].(string)
            na.AdminState = vna["admin_status"].(string)
            na.ActivateState = vna["activate_status"].(string)

            nas.Na = append(nas.Na, na)
        }

        return nas
    } else {
        fmt.Println(err.Error())
        return nil
    }
}

func (cc *ConfigClient) saveConfig() (string, error) {
    log.Infof("saveConfig: config save -y")
    return cc.execShell("config save -y")
}

func (cc *ConfigClient) getBgpNeighborAddresses() ([]*mt_proto.Value, error) {
    // make prefix
    prefix := new(gnmipb.Path)
    prefix.Target = configDBStr

    // make path
    pe := new(gnmipb.PathElem)
    pe.Name = "BGP_NEIGHBOR"

    path := new(gnmipb.Path)
    path.Elem = append(path.Elem, pe)

    paths := make([]*gnmipb.Path, 0)
    paths = append(paths, path)

    // get data
    db_client, err := NewDbClient(paths, prefix)
    if err == nil {
        return db_client.Get(nil)
    } else {
        return nil, err
    }
}

func (cc *ConfigClient) getInterfaceEthernetAddresses() ([]*mt_proto.Value, error) {
    // make prefix
    prefix := new(gnmipb.Path)
    prefix.Target = applDBStr

    // make path
    pe := new(gnmipb.PathElem)
    pe.Name = "INTF_TABLE"

    path := new(gnmipb.Path)
    path.Elem = append(path.Elem, pe)

    paths := make([]*gnmipb.Path, 0)
    paths = append(paths, path)

    // get data
    db_client, err := NewDbClient(paths, prefix)
    if err == nil {
        return db_client.Get(nil)
    } else {
        return nil, err
    }
}

func (cc *ConfigClient) getEntriesEntry(s string) (string, error) {
    log.Infof("getEntriesEntry: ip route " + s)
    return cc.execShell("ip route list " + s)
}

func (cc *ConfigClient) getRibTableEntriesFWD() (string, error) {
    log.Infof("getRibTableEntriesFWD: ip route list | awk '{ print $1 }' | grep '^[1-9]'")
    return cc.execShell("ip route list table main | awk '{ print $1 }' | grep '^[1-9]'")
}

func (cc *ConfigClient) getRibTableEntriesCTRL() (string, error) {
    log.Infof("getRibTableEntriesFWD: ip route list | awk '{ print $1 }' | grep '^[1-9]'")
    return cc.execShell("ip route list table main | awk '{ print $1 }' | grep '^[1-9]'")
}

func (cc *ConfigClient) getRibTableEntries() (string, error) {
    log.Infof("getRibTableEntries: ip route list | awk '{ print $1 }' | grep '^[1-9]'")
    return cc.execShell("ip route list | awk '{ print $1 }' | grep '^[1-9]'")
}

func (cc *ConfigClient) setBgpNeighborAddress(remoteAddrVal string, field string, val string) error {
    log.Infof("setBgpNeighborAddress: ", remoteAddrVal, " ", field, " ", val)

    // make prefix
    prefix := new(gnmipb.Path)
    prefix.Target = configDBStr

    // make path
    pe1 := new(gnmipb.PathElem)
    pe1.Name = "BGP_NEIGHBOR"

    pe2 := new(gnmipb.PathElem)
    pe2.Name = remoteAddrVal

    pe3 := new(gnmipb.PathElem)
    pe3.Name = field

    path := new(gnmipb.Path)
    path.Elem = append(path.Elem, pe1)
    path.Elem = append(path.Elem, pe2)
    path.Elem = append(path.Elem, pe3)

    paths := make([]*gnmipb.Path, 0)
    paths = append(paths, path)

    // get data
    db_client, err := NewDbClient(paths, prefix)
    if err == nil {
        _, err = db_client.Set(val)
        return err
    } else {
        return nil
    }
}

func (cc *ConfigClient) execShell(s string) (string, error) {
    cmd := exec.Command("/bin/bash", "-c", s)
    var out bytes.Buffer

    cmd.Stdout = &out
    err := cmd.Run()
    if err != nil {
        log.Infof("Error: %s", err.Error())
        return "", err
    } else {
        //log.Infof("out: %s", out.String())
        return out.String(), nil
    }
}

type yangDataReplaceFunc func(cc *ConfigClient, path *gnmipb.Path, val *gnmipb.TypedValue) error

type path2DataReplaceFunc struct {
    path string
    replaceFunction yangDataReplaceFunc
}

type yangDataGetFunc func(cc *ConfigClient, path *gnmipb.Path) (*gnmipb.TypedValue, error)

type path2DataGetFunc struct {
    path string
    getFunction yangDataGetFunc
}

var (
    YcTarget = "MTNOS"
    forwardPlaneStr = "forward_plane"
    controlPlaneStr = "control_plane"
    errorPrefix = "Error: "
    configDBStr = "CONFIG_DB"
    applDBStr = "APPL_DB"

    path2DataReplaceFuncTbl = []path2DataReplaceFunc{
        {
            path: "/local-routes/static-routes/static/",
            replaceFunction: staticRoutesStaticReplaceFunc,
        },
        {
            path: "/bgp/neighbors/neighbor/config/neighbor-addresses/address/activate-status",
            replaceFunction: bgpNeighborAddressesAddressActivateStatusReplaceFunc,
        },
    }

    path2DataGetFuncTbl = []path2DataGetFunc{
        {
            path: "/rib/table/entries/",
            getFunction: ribTableEntriesGetFunc,
        },
        {
            path: "/rib/table/entries/entry",
            getFunction: ribEntriesEntryGetFunc,
        },
        {
            path: "/interfaces/interface/ethernet/state/addresses",
            getFunction: interfaceEthernetAddressesGetFunc,
        },
        {
            path: "/bgp/neighbors/neighbor/state/neighbor-addresses",
            getFunction: bgpNeighborAddressesGetFunc,
        },
    }
)

type YangClient struct {
    cc *ConfigClient
    paths []*gnmipb.Path
}

func NewYangClient(paths []*gnmipb.Path) (*YangClient, error) {
    client := new(YangClient)
    client.cc = new(ConfigClient)
    client.paths = make([]*gnmipb.Path, len(paths))
    for i, p := range(paths) {
        client.paths[i] = p
    }
    return client, nil
}

func (yc *YangClient) CheckYangPathSet(path *string) (int, error) {
    err := fmt.Errorf("Err: path not found")
    path_no := 0

    for i, v := range(path2DataReplaceFuncTbl) {
        if v.path == *path {
            err = nil
            path_no = i
            break
        }
    }

    return path_no, err
}

func (yc *YangClient) CheckYangPathGet(path *string) (int, error) {
    err := fmt.Errorf("Err: path not found")
    path_no := 0

    for i, v := range(path2DataGetFuncTbl) {
        if v.path == *path {
            err = nil
            path_no = i
            break
        }
    }

    return path_no, err
}

func (yc *YangClient) SetPb(path *gnmipb.Path, val *gnmipb.TypedValue) error {
    fp := yc.getFullPath(path)
	log.Infof("Set: path %s", fp)

	path_no, err := yc.CheckYangPathSet(&fp)
	if err != nil {
		return err
    }

	return path2DataReplaceFuncTbl[path_no].replaceFunction(yc.cc, path, val)
}

func (yc *YangClient) GetPb() ([]*mt_proto.Value, error) {
    var err error
    var spbValues = make([]*mt_proto.Value, 0)

    for _, path := range(yc.paths) {
        var path_no int
        fp := yc.getFullPath(path)
        log.Infof("GetRequest GetPb fp: %s", fp)

        spbval := new(mt_proto.Value)
        spbval.Path = path

        path_no, err = yc.CheckYangPathGet(&fp)
        if err != nil {
            return nil, err
        }

        spbval.Val, err = path2DataGetFuncTbl[path_no].getFunction(yc.cc, path)

        if err != nil {
            log.Errorf("GetRequest GetPb err: %s", err.Error())
            spbValues = append(spbValues, spbval) // TODO, add error into return value
        } else {
            spbValues = append(spbValues, spbval)
        }
    }

    if err != nil {
        return spbValues, fmt.Errorf("Got error while getPb by path")
    } else {
        return spbValues, nil
    }
}

func (yc *YangClient) getFullPath(path *gnmipb.Path) string {
	fullPath := ""

	for _, pathElem := range(path.GetElem()) {
		fullPath += "/" + pathElem.GetName()
	}

    return fullPath
}

func (yc *YangClient) StreamRun(q *queue.PriorityQueue, stop chan struct{}, w *sync.WaitGroup) {
	return
}

func (yc *YangClient) PollRun(q *queue.PriorityQueue, poll chan struct{}, w *sync.WaitGroup) {
	return
}

func checkAllowedTypedValue(val *gnmipb.TypedValue) error {
    switch val.Value.(type) {
    case *gnmipb.TypedValue_ProtoBytes:
        return nil
    default:
        return fmt.Errorf("Only support TypedValue_ProtoBytes")
    }
}

func staticRoutesStaticReplaceFunc(cc *ConfigClient, path *gnmipb.Path, val *gnmipb.TypedValue) error {
    err := checkAllowedTypedValue(val)
    if err != nil {
		return err
	}

    srs := &mt_proto.StaticRoutesStatic{}
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

func bgpNeighborAddressesAddressActivateStatusReplaceFunc(cc *ConfigClient, path *gnmipb.Path, val *gnmipb.TypedValue) error {
    err := checkAllowedTypedValue(val)
    if err != nil {
		return err
    }

    remoteAddr := ""
    activateState := ""

    na := new(mt_proto.NeighborAddress)
    err = proto.Unmarshal(val.GetProtoBytes(), na)
	if err != nil {
		return err
    }

    remoteAddr = na.RemoteAddr
    activateState = na.ActivateState

    if remoteAddr == "" || (activateState != "up" && activateState != "down") {
        return fmt.Errorf("param %s %s error", remoteAddr, activateState)
    }

    err = cc.setBgpNeighborAddress(remoteAddr, "activate_status", activateState)

    if err != nil {
        return err
    }

    return nil
}

func ribTableEntriesGetFunc(cc *ConfigClient, path *gnmipb.Path) (*gnmipb.TypedValue, error) {
    log.Infof("ribTableEntriesGetFunc")

    out, err := cc.getRibTableEntries()

    if err != nil {
        log.Infof("Error: %s", err)
        return nil, err
    } else {
        //log.Infof("out: %s", out)

        ret := mt_proto.RibTableEntries {
            Entry: []*mt_proto.EntriesEntry {
                &mt_proto.EntriesEntry {
                    EntryDetail: out,
                },
            },
        }

        out, err := proto.Marshal(&ret)
        if err != nil {
            return nil, err
        }
        pbVal := &gnmipb.TypedValue{
            Value: &gnmipb.TypedValue_ProtoBytes{
                ProtoBytes: out,
            },
        }

        return pbVal, nil
    }
}

func ribEntriesEntryGetFunc(cc *ConfigClient, path *gnmipb.Path) (*gnmipb.TypedValue, error) {
    log.Infof("ribEntriesEntryGetFunc")

    var s string
    var ok bool
    for _, pathElem := range(path.GetElem()) {
		if pathElem.GetName() == "entry" && pathElem.GetKey() != nil {
            s, ok = pathElem.GetKey()["entryKey"]
            if ok {
                break;
            }
        }
    }

    if !ok {
        return nil, fmt.Errorf("No entryKey in entry elem")
    }

    var out string
    var err error
    if s == forwardPlaneStr {
        out, err = cc.getRibTableEntriesFWD()
    } else if s == controlPlaneStr {
        out, err = cc.getRibTableEntriesCTRL()
    } else {
        out, err = cc.getEntriesEntry(s)
    }

    if err != nil {
        return nil, err
    } else {
        //log.Infof("out: %s", out)

        ret := mt_proto.RibTableEntries {
            Entry: []*mt_proto.EntriesEntry {
                &mt_proto.EntriesEntry {
                    EntryDetail: out,
                },
            },
        }

        out, err := proto.Marshal(&ret)
        if err != nil {
            return nil, err
        }
        pbVal := &gnmipb.TypedValue{
            Value: &gnmipb.TypedValue_ProtoBytes{
                ProtoBytes: out,
            },
        }

        return pbVal, nil
    }
}

func interfaceEthernetAddressesGetFunc(cc *ConfigClient, path *gnmipb.Path) (*gnmipb.TypedValue, error) {
    log.Infof("interfaceEthernetAddressesGetFunc")

    var s string
    var ok bool
    for _, pathElem := range(path.GetElem()) {
		if pathElem.GetName() == "ethernet" && pathElem.GetKey() != nil {
            s, ok = pathElem.GetKey()["name"]
            if ok {
                break;
            }
        }
    }

    if !ok {
        return nil, fmt.Errorf("No name in Ethernet elem")
    }

    spbValues, err := cc.getInterfaceEthernetAddresses()

    if err != nil {
        return nil, err
    } else {
        addr := cc.parseIntfAddr(spbValues[0].GetVal().GetJsonIetfVal(), s)

        out, err := proto.Marshal(addr)
        if err != nil {
            return nil, err
        }
        pbVal := &gnmipb.TypedValue{
            Value: &gnmipb.TypedValue_ProtoBytes{
                ProtoBytes: out,
            },
        }

        return pbVal, nil
    }
}

func bgpNeighborAddressesGetFunc(cc *ConfigClient, path *gnmipb.Path) (*gnmipb.TypedValue, error) {
    log.Infof("bgpNeighborAddressesGetFunc")
    spbValues, err := cc.getBgpNeighborAddresses()

    if err != nil {
        return nil, err
    } else {
        nas := cc.parseBgpNa(spbValues[0].GetVal().GetJsonIetfVal())

        out, err := proto.Marshal(nas)
        if err != nil {
            return nil, err
        }
        pbVal := &gnmipb.TypedValue{
            Value: &gnmipb.TypedValue_ProtoBytes{
                ProtoBytes: out,
            },
        }

        return pbVal, nil
    }
}