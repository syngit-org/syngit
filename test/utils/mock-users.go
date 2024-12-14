package utils

import (
	"context"
	"fmt"
	"os"

	. "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestUser string

var (
	Admin   TestUser
	Sanji   TestUser
	Chopper TestUser
	Luffy   TestUser
)

var Users []TestUser

type SyngitTestUsersClientset struct {
	admin  *Clientset
	config *rest.Config
}

func (tu *SyngitTestUsersClientset) asWithError(username TestUser) (*Clientset, error) {
	if username != Admin {
		return tu.kImpersonate(username)
	} else {
		return tu.admin, nil
	}
}

func (tu *SyngitTestUsersClientset) KAs(username TestUser) *Clientset {
	cs, err := tu.asWithError(username)
	if err != nil {
		return tu.admin
	} else {
		return cs
	}
}

func (tu *SyngitTestUsersClientset) CAs(username TestUser) client.Client {
	client, _ := tu.impersonate(username)
	return client
}

func (tu *SyngitTestUsersClientset) As(username TestUser) CustomClient {
	client, _ := tu.impersonate(username)
	customClient := CustomClient{
		ctx:    context.TODO(),
		client: client,
	}
	return customClient
}

func (tu *SyngitTestUsersClientset) Initialize() error {
	Admin = TestUser(os.Getenv("ADMIN_USERNAME"))
	Sanji = TestUser(os.Getenv("SANJI_USERNAME"))
	Chopper = TestUser(os.Getenv("CHOPPER_USERNAME"))
	Luffy = TestUser(os.Getenv("LUFFY_USERNAME"))

	Users = []TestUser{Sanji, Chopper, Luffy}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = fmt.Sprintf("%s/.kube/config", home)
		}
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}
	tu.config = config

	tu.admin, err = NewForConfig(tu.config)

	return err
}

func (tu *SyngitTestUsersClientset) kImpersonate(username TestUser) (*Clientset, error) {
	tu.config.Impersonate = rest.ImpersonationConfig{
		UserName: string(username),
		Groups:   []string{"system:authenticated"}, // Ensure user is authenticated
	}

	clientset, err := NewForConfig(tu.config)

	return clientset, err
}

func (tu *SyngitTestUsersClientset) impersonate(username TestUser) (client.Client, error) {
	tu.config.Impersonate = rest.ImpersonationConfig{
		UserName: string(username),
		Groups:   []string{"system:authenticated"}, // Ensure user is authenticated
	}

	return client.New(tu.config, client.Options{Scheme: scheme.Scheme})
}
