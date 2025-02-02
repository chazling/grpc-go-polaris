/**
 * Tencent is pleased to support the open source community by making CL5 available.
 *
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 *
 * Licensed under the BSD 3-Clause License (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://opensource.org/licenses/BSD-3-Clause
 *
 * Unless required by applicable law or agreed to in writing, software distributed
 * under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
 * CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 */

package main

import (
	"context"
	"fmt"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc"

	polaris "github.com/polarismesh/grpc-go-polaris"
	hello "github.com/polarismesh/grpc-go-polaris/sample/model/grpc"
)

// go build -mod=vendor
// 命令行格式：./rpcClient <target> <sendCount> <sendInterval> <metadata k1:metadata v1,metadata k2:metadata v2,...>
// ./rpcServer Development/yourService sendCount sendInterval yourMetadata

var (
	regexPolaris, _ = regexp.Compile("^(Development|Production|Pre-release|Test)/([a-zA-Z0-9_:.-]{1,128})$")
)

func main() {
	namespace, service, target, sendCount, sendInterval, metadata := processArgs()

	//创建并设置 Polaris 配置对象
	configuration := api.NewConfiguration()
	//设置北极星server的地址
	configuration.GetGlobal().GetServerConnector().SetAddresses([]string{"127.0.0.1:8090"})
	//设置连接北极星server的超时时间
	configuration.GetGlobal().GetServerConnector().SetConnectTimeout(2 * time.Second)
	//设置consumer关闭全死全活，可选
	configuration.GetConsumer().GetServiceRouter().SetEnableRecoverAll(false)
	//设置服务路由相关配置，使用元数据路由以及规则路由
	configuration.GetConsumer().GetServiceRouter().SetChain(
		[]string{config.DefaultServiceRouterDstMeta, config.DefaultServiceRouterRuleBased})

	//使用配置获取 Polaris SDK 对象
	//Polaris Consumer API
	consumer, err := api.NewConsumerAPIByConfig(configuration)
	if err != nil {
		log.Fatalf("api.NewConsumerAPIByConfig err(%v)", err)
	}
	defer consumer.Destroy()

	//初始化并注册 Polaris Resolver Builder
	polaris.Init(polaris.Conf{
		PolarisConsumer: consumer,
		SyncInterval:    time.Second * time.Duration(sendInterval),
		Metadata:        metadata,                    //需要进行元数据路由的元数据，可选
		SourceService:   &model.ServiceInfo{          //通过SourceService属性设置主调服务的过滤标签，可选
			Namespace: namespace,                     //主调的namespace
			Service:   service,                       //主调的service
			Metadata: map[string]string{              //用于匹配路由规则
				"flag": "test",
			},
		},
		HeaderPrefix: []string{"naming_","tracing_"},  //用于过滤作为路由规则的gRPC Header，可选
	})

	//grpc客户端连接获取
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := grpc.DialContext(ctx, fmt.Sprintf("polaris://%s", target),
		[]grpc.DialOption{
			grpc.WithInsecure(),
		}...)
	if err != nil {
		panic(err)
	}

	//grpc客户端调用
	rpcClient := hello.NewHelloClient(conn)
	for i := 0; i < sendCount; i++ {
		resp, err := rpcClient.SayHello(ctx, &hello.HelloRequest{Name: "polaris"})
		log.Printf("send message, index (%d) resp1 (%v), err(%v)", i, resp, err)

		<-time.After(1500 * time.Millisecond)
	}
}

//解析启动参数
func processArgs() (string, string, string, int, int, map[string]string) {
	params := os.Args[1:]
	if len(params) < 5 {
		log.Fatalf("using %s <namespace> <service> <target> <sendCount> <sendInterval> "+
			"<metadata k1:metadata v1,metadata k2:metadata v2,...>", os.Args[0])
	}

	namespace := params[0]
	service := params[1]
	target := params[2]
	if !regexPolaris.MatchString(target) {
		log.Fatalf("using invalid target: %s", os.Args[0])
	}
	sendCount, err := strconv.Atoi(params[3])
	if nil != err {
		log.Fatalf("fail to convert sendCount %s to int, err %v", params[1], err)
	}
	sendInterval, err := strconv.Atoi(params[4])
	if nil != err {
		log.Fatalf("fail to convert sendInterval %s to int, err %v", params[2], err)
	}
	if len(params) > 5 {
		metadata, err := parseMetadata(params[5])
		if nil != err {
			log.Fatalf("fail to parse metadata string %s, err %v", params[3], err)
		}
		return namespace, service, target, sendCount, sendInterval, metadata
	}

	return namespace, service, target, sendCount, sendInterval, nil
}

//解析服务元数据
func parseMetadata(metadataStr string) (map[string]string, error) {
	strMetadata := strings.Split(metadataStr, ",")
	metadata := make(map[string]string, len(strMetadata))
	for _, str := range strMetadata {
		if len(str) == 0 {
			continue
		}
		metadataKv := strings.Split(str, ":")
		if len(metadataKv) != 2 {
			return nil, fmt.Errorf("invalid kv pair str %s", str)
		}
		metadata[metadataKv[0]] = metadataKv[1]
	}
	return metadata, nil
}
