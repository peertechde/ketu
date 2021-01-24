package template

import (
	"context"
	"encoding/json"
	"fmt"
	"text/template"

	"go.etcd.io/etcd/clientv3"
)

type config struct {
	client *clientv3.Client
	set    *Set
}

func Funcs(config config) template.FuncMap {
	return template.FuncMap{
		"get": getValue(config.client, config.set),

		"parseJSON": parseJSON,
	}
}

// todo: value could be already in the set (cache); check for it
// todo: store the revision
func getValue(client *clientv3.Client, set *Set) func(string) (string, error) {
	return func(s string) (string, error) {
		resp, err := client.Get(context.Background(), s)
		if err != nil {
			return "", err
		}
		if resp.Count == 0 {
			return "", fmt.Errorf("key not found")
		}
		set.Put(s, resp.Kvs[0].Value)

		return string(resp.Kvs[0].Value), nil
	}
}

func parseJSON(s string) (interface{}, error) {
	if s == "" {
		return map[string]interface{}{}, nil
	}

	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return nil, err
	}
	return data, nil
}
