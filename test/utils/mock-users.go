package utils

import (
	"context"
	"os"

	. "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestUser string

var (
	Admin   TestUser
	Sanji   TestUser
	Chopper TestUser
	Luffy   TestUser
	Brook   TestUser
)

var Users map[string]TestUser

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

func (tu *SyngitTestUsersClientset) Initialize(cfg *rest.Config) error {
	// Platform engineer : admin in the whole cluster
	Admin = TestUser(os.Getenv("ADMIN_USERNAME"))
	// DevOps: admin in the test namespace
	Luffy = TestUser(os.Getenv("LUFFY_USERNAME"))
	// DevOps: admin in the test namespace
	Chopper = TestUser(os.Getenv("CHOPPER_USERNAME"))
	// DevOps : admin in the test namespace
	Sanji = TestUser(os.Getenv("SANJI_USERNAME"))
	// Limited DevOps : CRUD only on syngit objects that belongs to him
	//   Can get secrets (to authenticate with the RemoteUser)
	Brook = TestUser(os.Getenv("BROOK_USERNAME"))

	Users = map[string]TestUser{}
	Users[os.Getenv("LUFFY_USERNAME")] = Luffy
	Users[os.Getenv("CHOPPER_USERNAME")] = Chopper
	Users[os.Getenv("SANJI_USERNAME")] = Sanji
	Users[os.Getenv("BROOK_USERNAME")] = Brook

	tu.config = cfg

	var err error
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
