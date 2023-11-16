package main

import (
	"bytes"
	// "flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/remotecommand"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ExecCmdExample is a function to execute a command in a pod.
func ExecCmdExample(client kubernetes.Interface, config *rest.Config, podName string, command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := []string{"sh", "-c", command}
	req := client.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
		Namespace("default").SubResource("exec")
	option := &v1.PodExecOptions{
		Command: cmd,
		Stdin:   true,
		Stdout:  true,
		Stderr:  true,
		TTY:     true,
	}
	if stdin == nil {
		option.Stdin = false
	}
	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return err
	}
	return nil
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	var msg struct {
		PodName string `json:"podName"`
		Command string `json:"command"`
	}

	err = conn.ReadJSON(&msg)
	if err != nil {
		log.Println(err)
		return
	}

	kubeconfig, err := getKubeconfig()
	if err != nil {
		log.Println(err)
		return
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Println(err)
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Println(err)
		return
	}

	var stdin io.Reader
	var stdout, stderr bytes.Buffer

	err = ExecCmdExample(clientset, config, msg.PodName, msg.Command, stdin, &stdout, &stderr)
	if err != nil {
		log.Printf("Error executing command: %v\n", err)
		return
	}

	output := stdout.String() + stderr.String()
	err = conn.WriteMessage(websocket.TextMessage, []byte(output))
	if err != nil {
		log.Println(err)
	}
}

func getKubeconfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home + "/.kube/config", nil
}

func main() {
	http.HandleFunc("/ws", handleWS)
	port := 4000
	log.Printf("WebSocket server listening on :%d...\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}