package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

func main() {
	sockets := "/run/user/1000/podman/podman.sock"

	httpc := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sockets)
			},
		},
	}

	req, err := http.NewRequest("GET", "http://localhost/containers/json", nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := httpc.Do(req)
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}

	var res []map[string]interface{}
	json.Unmarshal(body, &res)

	for _, c := range res {
		fmt.Println(c["Names"])
		fmt.Println(c["Ports"].Interface()["PrivatePort"])
	}
}
