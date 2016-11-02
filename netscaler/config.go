/*
Copyright 2016 Citrix Systems, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package netscaler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chiradeep/go-nitro/config/basic"
	"github.com/chiradeep/go-nitro/config/cs"
	"github.com/chiradeep/go-nitro/config/lb"
	"github.com/chiradeep/go-nitro/netscaler"
	"log"
	"sort"
	"strconv"
	"strings"
)

func GenerateLbName(namespace string, host string) string {
	lbName := "lb_" + strings.Replace(host, ".", "_", -1)
	return lbName
}

func GenerateCsVserverName(namespace string, ingressName string) string {
	csv := "cs_" + namespace + "_" + ingressName
	return csv
}

func GeneratePolicyName(namespace string, host string, path string) string {
	path_ := path
	if path == "" {
		path_ = "nilpath"
	}
	path_ = strings.Replace(path_, "/", "_", -1)
	host = strings.Replace(host, ".", "_", -1)

	policyName := host + "-" + path_ + "_policy"
	return policyName
}

func GenerateActionName(namespace string, host string, path string) string {
	path_ := path
	if path == "" {
		path_ = "nilpath"
	}
	path_ = strings.Replace(path_, "/", "_", -1)
	host = strings.Replace(host, ".", "_", -1)
	actionName := host + "-" + path_ + "_action"
	return actionName
}

func DeleteService(sname string) {
	client, _ := netscaler.NewNitroClientFromEnv()
	err := client.DeleteResource(netscaler.Service.Type(), sname)
	if err != nil {
		log.Println(fmt.Sprintf("Failed to delete service %s err=%s", sname, err))
	}
}

func AddAndBindService(lbName string, sname string, IpPort string) {
	//create a Netscaler Service that represents the Kubernetes service
	client, _ := netscaler.NewNitroClientFromEnv()
	ep_ip_port := strings.Split(IpPort, ":")
	servicePort, _ := strconv.Atoi(ep_ip_port[1])
	nsService := basic.Service{
		Name:        sname,
		Ip:          ep_ip_port[0],
		Servicetype: "HTTP",
		Port:        servicePort,
	}
	_, err := client.AddResource(netscaler.Service.Type(), sname, &nsService)

	if err != nil {
		binding := lb.Lbvserverservicebinding{
			Name:        lbName,
			Servicename: sname,
		}
		_ = client.BindResource(netscaler.Lbvserver.Type(), lbName, netscaler.Service.Type(), sname, &binding)
	}
}

func ConfigureContentVServer(namespace string, csvserverName string, domainName string, path string, serviceIp string,
	serviceName string, servicePort int, priority int, svcname_refcount map[string]int) string {
	lbName := GenerateLbName(namespace, domainName)
	policyName := GeneratePolicyName(namespace, domainName, path)
	actionName := GenerateActionName(namespace, domainName, path)
	client, _ := netscaler.NewNitroClientFromEnv()

	//create a Netscaler Service that represents the Kubernetes service
	nsService := basic.Service{
		Name:        serviceName,
		Ip:          serviceIp,
		Servicetype: "HTTP",
		Port:        servicePort,
	}
	_, _ = client.AddResource(netscaler.Service.Type(), serviceName, &nsService)

	_, present := svcname_refcount[serviceName]
	if present {
		svcname_refcount[serviceName]++
	} else {
		svcname_refcount[serviceName] = 1
	}

	//create a Netscaler "lbvserver" to front the service
	nsLB := lb.Lbvserver{
		Name:        lbName,
		Servicetype: "HTTP",
	}
	_, _ = client.AddResource(netscaler.Lbvserver.Type(), lbName, &nsLB)

	//bind the lb to the service
	binding := lb.Lbvserverservicebinding{
		Name:        lbName,
		Servicename: serviceName,
	}
	_ = client.BindResource(netscaler.Lbvserver.Type(), lbName, netscaler.Service.Type(), serviceName, &binding)

	//create a content switch action to switch to the lb
	csAction := cs.Csaction{
		Name:            actionName,
		Targetlbvserver: lbName,
	}
	_, _ = client.AddResource(netscaler.Csaction.Type(), actionName, &csAction)

	//create a content switch policy to use the action
	var rule string
	if path != "" {
		rule = fmt.Sprintf("HTTP.REQ.HOSTNAME.EQ(\"%s\") && HTTP.REQ.URL.PATH.EQ(\"%s\")", domainName, path)
	} else {
		rule = fmt.Sprintf("HTTP.REQ.HOSTNAME.EQ(\"%s\")", domainName)
	}
	csPolicy := cs.Cspolicy{
		Policyname: policyName,
		Rule:       rule,
		Action:     actionName,
	}
	_, _ = client.AddResource(netscaler.Cspolicy.Type(), policyName, &csPolicy)

	//bind the content switch policy to the content switching vserver
	binding2 := cs.Csvservercspolicybinding{
		Name:       csvserverName,
		Policyname: policyName,
		Priority:   priority,
		Bindpoint:  "REQUEST",
	}
	_ = client.BindResource(netscaler.Csvserver.Type(), csvserverName, netscaler.Cspolicy.Type(), policyName, &binding2)

	return lbName
}

func CreateContentVServer(csvserverName string, vserverIp string, vserverPort int, protocol string) error {
	client, _ := netscaler.NewNitroClientFromEnv()
	cs := cs.Csvserver{
		Name:        csvserverName,
		Ipv46:       vserverIp,
		Servicetype: protocol,
		Port:        vserverPort,
	}
	_, _ = client.AddResource(netscaler.Csvserver.Type(), csvserverName, &cs)
	return nil
}

func DeleteContentVServer(csvserverName string, svcname_refcount map[string]int, lbName_map map[string]int) {
	client, _ := netscaler.NewNitroClientFromEnv()
	policyNames, _ := ListBoundPolicies(csvserverName)

	for _, policyName := range policyNames {
		//unbind the content switch policy from the content switching vserver
		err := client.UnbindResource(netscaler.Csvserver.Type(), csvserverName, netscaler.Cspolicy.Type(), policyName, "policyName")
		if err != nil {
			log.Fatal(fmt.Sprintf("Failed to unbind Content Switching Policy %s fromo Content Switching VServer %s, err=%s", policyName, csvserverName, err))
			continue
		}

		//find the action name from the policy
		actionName := ListPolicyAction(policyName)

		err = client.DeleteResource(netscaler.Cspolicy.Type(), policyName)
		if err != nil {
			log.Printf("Failed to delete Content Switching Policy %s, err=%s", policyName, err)
			continue
		}
		//find the lb name associated with the action
		lbName, err := ListLbVserverForAction(actionName)

		if err != nil {
			log.Printf("Failed to obtain lb name for cs action %s", actionName)
			continue
		}
		//delete content switch action that switches to the lb
		err = client.DeleteResource(netscaler.Csaction.Type(), actionName)
		if err != nil {
			log.Fatal(fmt.Sprintf("Failed to delete Content Switching Action %s for LB %s err=%s", actionName, lbName, err))
			return
		}

		//find the service names that the LB is bound to
		serviceNames, err := ListBoundServicesForLB(lbName)
		if err != nil {
			log.Printf("Failed to retrieve services bound to LB " + lbName)
			continue
		}
		for _, sname := range serviceNames {
			err = client.UnbindResource(netscaler.Lbvserver.Type(), lbName, netscaler.Service.Type(), sname, "servicename")

			if err != nil {
				log.Fatal(fmt.Sprintf("Failed to unbind svc %s from lb %s, err=%s", sname, lbName, err))
				continue
			}
		}

		//delete  "lbvserver" that fronts the service
		err = client.DeleteResource(netscaler.Lbvserver.Type(), lbName)

		if lbName_map != nil {
			delete(lbName_map, lbName)
		}

		//Delete the Netscaler Services
		for _, sname := range serviceNames {

			_, present := svcname_refcount[sname]
			if present {
				svcname_refcount[sname]--
			}

			if svcname_refcount[sname] == 0 {
				delete(svcname_refcount, sname)
				err = client.DeleteResource(netscaler.Service.Type(), sname)
				if err != nil {
					log.Println(fmt.Sprintf("Failed to delete service %s err=%s", sname, err))
					continue
				}
			}
		}
	}
	_ = client.DeleteResource(netscaler.Csvserver.Type(), csvserverName)
}

func FindContentVserver(csvserverName string) bool {
	client, _ := netscaler.NewNitroClientFromEnv()
	return client.ResourceExists(netscaler.Csvserver.Type(), csvserverName)
}

func ListContentVservers() []string {
	result := []string{}

	body, err := listResource("csvserver", "")
	if err != nil {
		log.Printf("No csvservers found")
		return result
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		log.Println("Failed to unmarshal Netscaler Response!")
		return []string{}
	}
	if data["csvserver"] == nil {
		log.Printf("No csvservers found")
		return result
	}

	csvs := data["csvserver"].([]interface{})
	for _, c := range csvs {
		csvserver := c.(map[string]interface{})
		csname := csvserver["name"].(string)

		result = append(result, csname)
	}
	return result

}

func ListBoundPolicies(csvserverName string) ([]string, []int) {
	result, err := listBoundResources(csvserverName, "csvserver", "cspolicy", "", "")
	ret1 := []string{}
	ret2 := []int{}
	if err != nil {
		log.Println("No bindings for CS Vserver %s", csvserverName)
		return ret1, ret2
	}
	var data map[string]interface{}
	if err := json.Unmarshal(result, &data); err != nil {
		log.Println("Failed to unmarshal Netscaler Response!")
		return ret1, ret2
	}

	if data["csvserver_cspolicy_binding"] == nil {
		return ret1, ret2
	}

	bindings := data["csvserver_cspolicy_binding"].([]interface{})
	for _, b := range bindings {
		binding := b.(map[string]interface{})
		pname := binding["policyname"].(string)
		prio, err := strconv.Atoi(binding["priority"].(string))
		if err != nil {
			continue
		}
		ret1 = append(ret1, pname)
		ret2 = append(ret2, prio)
	}
	sort.Ints(ret2)
	return ret1, ret2
}

func ListBoundPolicy(csvserverName string, policyName string) map[string]int {
	result, err := listBoundResources(csvserverName, "csvserver", "cspolicy", "policyname", policyName)
	if err != nil {
		log.Println("No bindings for CS Vserver %s policy %", csvserverName, policyName)
		return map[string]int{}
	}
	var data map[string]interface{}
	if err := json.Unmarshal(result, &data); err != nil {
		log.Println("Failed to unmarshal Netscaler Response!")
		return map[string]int{}
	}

	ret := make(map[string]int)
	if data["csvserver_cspolicy_binding"] == nil {
		return ret
	}
	bindings := data["csvserver_cspolicy_binding"].([]interface{})
	for _, b := range bindings {
		binding := b.(map[string]interface{})
		pname := binding["policyname"].(string)
		prio := binding["priority"].(string)
		ret[pname], _ = strconv.Atoi(prio)
	}
	return ret
}

func ListPolicyAction(policyName string) string {
	result, err := listResource("cspolicy", policyName)
	if err != nil {
		log.Println("No policy %s", policyName)
		return ""
	}
	var data map[string]interface{}
	if err := json.Unmarshal(result, &data); err != nil {
		log.Println("Failed to unmarshal Netscaler Response!")
		return ""
	}

	policy := data["cspolicy"].([]interface{})[0]
	return policy.(map[string]interface{})["action"].(string)
}

func ListLbVserverForAction(actionName string) (string, error) {
	result, err := listResource("csaction", actionName)
	if err != nil {
		log.Println("No action %s", actionName)
		return "", errors.New("No action " + actionName)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(result, &data); err != nil {
		log.Println("Failed to unmarshal Netscaler Response!")
		return "", errors.New("Failed to unmarshal Netscaler response")
	}

	action := data["csaction"].([]interface{})[0]
	return action.(map[string]interface{})["targetlbvserver"].(string), nil
}

func DeleteCsPolicies(csvserverName string, policyNames []string) {

	for _, policyName := range policyNames {
		//unbind the content switch policy from the content switching vserver
		resourceType := "csvserver_cspolicy_binding"
		_, err := unbindResource(resourceType, csvserverName, "policyName", policyName)
		if err != nil {
			log.Fatal(fmt.Sprintf("Failed to unbind Content Switching Policy %s fromo Content Switching VServer %s, err=%s", policyName, csvserverName, err))
			return
		}

		resourceType = "cspolicy"
		//if there was an action in the policy, find that action
		action := ListPolicyAction(policyName)

		//delete the content switch policy that uses the action
		resourceType = "cspolicy"

		_, err = deleteResource(resourceType, policyName)
		if err != nil {
			log.Fatal(fmt.Sprintf("Failed to delete Content Switching Policy %s, err=%s", policyName, err))
			return
		}

		_, err = deleteResource("csaction", action)

		if err != nil {
			log.Fatal(fmt.Sprintf("Failed to delete Content Switching Policy Action%s, err=%s", action, err))
			return
		}

	}
}

func ListBoundServicesForLB(lbName string) ([]string, error) {
	result, err := listBoundResources(lbName, "lbvserver", "service", "", "")
	ret := []string{}
	if err != nil {
		log.Println("No bindings for LB Vserver %s", lbName)
		return ret, nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal(result, &data); err != nil {
		log.Println("Failed to unmarshal Netscaler Response!")
		return ret, errors.New("Failed to unmarshal Netscaler response")
	}

	if data["lbvserver_service_binding"] == nil {
		return ret, nil
	}

	bindings := data["lbvserver_service_binding"].([]interface{})
	for _, b := range bindings {
		binding := b.(map[string]interface{})
		sname := binding["servicename"].(string)

		ret = append(ret, sname)
	}
	return ret, nil
}
