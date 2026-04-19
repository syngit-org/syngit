package interceptor

import (
	"fmt"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	authenticationv1 "k8s.io/api/authentication/v1"
)

func IsBypassSubject(
	userInfo authenticationv1.UserInfo,
	remoteSyncer syngit.RemoteSyncer,
) (bool, error) {
	isBypassSubject := false

	userCountLoop := 0 // Prevent non-unique name attack
	for _, subject := range remoteSyncer.Spec.BypassInterceptionSubjects {
		// The subject name can not be unique -> in specific conditions, a commit can be done as another user
		// Need to be studied
		if subject.Name == userInfo.Username {
			isBypassSubject = true
			userCountLoop++
		}
	}

	if userCountLoop > 1 {
		return isBypassSubject, syngiterrors.NewTooMuchSubject(
			fmt.Sprintf("the name of the user is not unique (%d); this version of the operator work with the name as unique identifier for users", userCountLoop),
		)
	}

	return isBypassSubject, nil
}
