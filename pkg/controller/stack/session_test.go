package stack

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/pulumi/pulumi-kubernetes-operator/pkg/apis/pulumi/v1alpha1"
	"github.com/pulumi/pulumi/sdk/v2/go/x/auto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	secretName = "fake-secret"
	namespace  = "test"
)

type GitAuthTestSuite struct {
	suite.Suite
	f string
}

func (suite *GitAuthTestSuite) SetupTest() {
	f, err := ioutil.TempFile("", "")
	suite.NoError(err)
	defer f.Close()
	f.WriteString("super secret")
	suite.f = f.Name()
	os.Setenv("SECRET3", "so secret")
}

func (suite *GitAuthTestSuite) AfterTest() {
	if suite.f != "" {
		os.Remove(suite.f)
	}
	os.Unsetenv("SECRET3")
	suite.T().Log("Cleaned up")
}

func TestSuite(t *testing.T) {
	suite.Run(t, new(GitAuthTestSuite))
}

func (suite *GitAuthTestSuite) TestSetupGitAuthWithSecrets() {
	t := suite.T()
	logger := log.WithValues("Request.Test", "TestSetupGitAuthWithSecrets")

	sshPrivateKey := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sshPrivateKey",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"sshPrivateKey": []byte("very secret key"),
		},
		Type: "Opaque",
	}
	sshPrivateKeyWithPassword := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sshPrivateKeyWithPassword",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"sshPrivateKey": []byte("very secret key"),
			"password":      []byte("moar secret password"),
		},
		Type: "Opaque",
	}
	accessToken := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "accessToken",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"accessToken": []byte("super secret access token"),
		},
		Type: "Opaque",
	}
	basicAuth := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basicAuth",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte("not so secret username"),
			"password": []byte("very secret password"),
		},
		Type: "Opaque",
	}
	basicAuthWithoutPassword := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basicAuthWithoutPassword",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte("not so secret username"),
		},
		Type: "Opaque",
	}
	client := fake.NewFakeClientWithScheme(scheme.Scheme,
		sshPrivateKey, sshPrivateKeyWithPassword, accessToken, basicAuth, basicAuthWithoutPassword)

	for _, test := range []struct {
		name          string
		gitAuthSecret string
		expected      *auto.GitAuth
		err           error
	}{
		{
			name:          "InvalidSecretName",
			gitAuthSecret: "MISSING",
			err:           fmt.Errorf("secrets \"MISSING\" not found"),
		},
		{
			name:          "ValidSSHPrivateKey",
			gitAuthSecret: sshPrivateKey.Name,
			expected: &auto.GitAuth{
				SSHPrivateKey: "very secret key",
			},
		},
		{
			name:          "ValidSSHPrivateKeyWithPassword",
			gitAuthSecret: sshPrivateKeyWithPassword.Name,
			expected: &auto.GitAuth{
				SSHPrivateKey: "very secret key",
				Password:      "moar secret password",
			},
		},
		{
			name:          "ValidAccessToken",
			gitAuthSecret: accessToken.Name,
			expected: &auto.GitAuth{
				PersonalAccessToken: "super secret access token",
			},
		},
		{
			name:          "ValidBasicAuth",
			gitAuthSecret: basicAuth.Name,
			expected: &auto.GitAuth{
				Username: "not so secret username",
				Password: "very secret password",
			},
		},
		{
			name:          "BasicAuthWithoutPassword",
			gitAuthSecret: basicAuthWithoutPassword.Name,
			err:           errors.New("missing 'password' secret entry"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := newReconcileStackSession(logger, v1alpha1.StackSpec{GitAuthSecret: test.gitAuthSecret}, client, namespace)
			gitAuth, err := session.SetupGitAuth()
			if test.err != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.expected, gitAuth)
		})
	}
}

func (suite *GitAuthTestSuite) TestSetupGitAuthWithRefs() {
	t := suite.T()
	logger := log.WithValues("Request.Test", "TestSetupGitAuthWithRefs")

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"SECRET1": []byte("very secret"),
			"SECRET2": []byte("moar secret"),
		},
		Type: "Opaque",
	}

	client := fake.NewFakeClientWithScheme(scheme.Scheme, secret)

	for _, test := range []struct {
		name     string
		gitAuth  *v1alpha1.GitAuthConfig
		expected *auto.GitAuth
		err      error
	}{
		{
			name:     "NilGitAuth",
			expected: &auto.GitAuth{},
		},
		{
			name:    "EmptyGitAuth",
			gitAuth: &v1alpha1.GitAuthConfig{},
			err:     fmt.Errorf("gitAuth config must specify exactly one of 'personalAccessToken', 'sshPrivateKey' or 'basicAuth'"),
		},
		{
			name: "GitAuthValidSecretReference",
			gitAuth: &v1alpha1.GitAuthConfig{
				PersonalAccessToken: &v1alpha1.ResourceRef{
					SelectorType: v1alpha1.ResourceSelectorSecret,
					ResourceSelector: v1alpha1.ResourceSelector{
						SecretRef: &v1alpha1.SecretSelector{
							Namespace: namespace,
							Name:      secret.Name,
							Key:       "SECRET1",
						},
					},
				},
			},
			expected: &auto.GitAuth{
				PersonalAccessToken: "very secret",
			},
		},
		{
			name: "GitAuthValidFileReference",
			gitAuth: &v1alpha1.GitAuthConfig{
				PersonalAccessToken: &v1alpha1.ResourceRef{
					SelectorType: v1alpha1.ResourceSelectorFS,
					ResourceSelector: v1alpha1.ResourceSelector{
						FileSystem: &v1alpha1.FSSelector{
							Path: suite.f,
						},
					},
				},
			},
			expected: &auto.GitAuth{
				PersonalAccessToken: "super secret",
			},
		},
		{
			name: "GitAuthInvalidFileReference",
			gitAuth: &v1alpha1.GitAuthConfig{
				PersonalAccessToken: &v1alpha1.ResourceRef{
					SelectorType: v1alpha1.ResourceSelectorFS,
					ResourceSelector: v1alpha1.ResourceSelector{
						FileSystem: &v1alpha1.FSSelector{
							Path: "/tmp/!@#@!#",
						},
					},
				},
			},
			err: fmt.Errorf("open /tmp/!@#@!#: no such file or directory"),
		},
		{
			name: "GitAuthValidEnvVarReference",
			gitAuth: &v1alpha1.GitAuthConfig{
				PersonalAccessToken: &v1alpha1.ResourceRef{
					SelectorType: v1alpha1.ResourceSelectorEnv,
					ResourceSelector: v1alpha1.ResourceSelector{
						Env: &v1alpha1.EnvSelector{
							Name: "SECRET3",
						},
					},
				},
			},
			expected: &auto.GitAuth{
				PersonalAccessToken: "so secret",
			},
		},
		{
			name: "GitAuthInvalidEnvReference",
			gitAuth: &v1alpha1.GitAuthConfig{
				PersonalAccessToken: &v1alpha1.ResourceRef{
					SelectorType: v1alpha1.ResourceSelectorEnv,
					ResourceSelector: v1alpha1.ResourceSelector{
						Env: &v1alpha1.EnvSelector{
							Name: "MISSING",
						},
					},
				},
			},
			err: fmt.Errorf("missing value for environment variable: MISSING"),
		},
		{
			name: "GitAuthValidSSHAuthWithoutPassword",
			gitAuth: &v1alpha1.GitAuthConfig{
				SSHAuth: &v1alpha1.SSHAuth{
					SSHPrivateKey: v1alpha1.ResourceRef{
						SelectorType: v1alpha1.ResourceSelectorSecret,
						ResourceSelector: v1alpha1.ResourceSelector{
							SecretRef: &v1alpha1.SecretSelector{
								Namespace: namespace,
								Name:      secret.Name,
								Key:       "SECRET1",
							},
						},
					},
				},
			},
			expected: &auto.GitAuth{
				SSHPrivateKey: "very secret",
			},
		},
		{
			name: "GitAuthValidSSHAuthWithPassword",
			gitAuth: &v1alpha1.GitAuthConfig{
				SSHAuth: &v1alpha1.SSHAuth{
					SSHPrivateKey: v1alpha1.ResourceRef{
						SelectorType: v1alpha1.ResourceSelectorSecret,
						ResourceSelector: v1alpha1.ResourceSelector{
							SecretRef: &v1alpha1.SecretSelector{
								Namespace: namespace,
								Name:      secret.Name,
								Key:       "SECRET1",
							},
						},
					},
					Password: &v1alpha1.ResourceRef{
						SelectorType: v1alpha1.ResourceSelectorSecret,
						ResourceSelector: v1alpha1.ResourceSelector{
							SecretRef: &v1alpha1.SecretSelector{
								Namespace: namespace,
								Name:      secret.Name,
								Key:       "SECRET2",
							},
						},
					},
				},
			},
			expected: &auto.GitAuth{
				SSHPrivateKey: "very secret",
				Password:      "moar secret",
			},
		},
		{
			name: "GitAuthInvalidSSHAuthWithPassword",
			gitAuth: &v1alpha1.GitAuthConfig{
				SSHAuth: &v1alpha1.SSHAuth{
					SSHPrivateKey: v1alpha1.ResourceRef{
						SelectorType: v1alpha1.ResourceSelectorSecret,
						ResourceSelector: v1alpha1.ResourceSelector{
							SecretRef: &v1alpha1.SecretSelector{
								Namespace: namespace,
								Name:      secret.Name,
								Key:       "SECRET1",
							},
						},
					},
					Password: &v1alpha1.ResourceRef{
						SelectorType: v1alpha1.ResourceSelectorSecret,
						ResourceSelector: v1alpha1.ResourceSelector{
							SecretRef: &v1alpha1.SecretSelector{
								Namespace: namespace,
								Name:      secret.Name,
								Key:       "MISSING",
							},
						},
					},
				},
			},
			err: fmt.Errorf("resolving gitAuth SSH password: No key MISSING found in secret test/fake-secret"),
		},
		{
			name: "GitAuthValidBasicAuth",
			gitAuth: &v1alpha1.GitAuthConfig{
				BasicAuth: &v1alpha1.BasicAuth{
					UserName: v1alpha1.ResourceRef{
						SelectorType: v1alpha1.ResourceSelectorSecret,
						ResourceSelector: v1alpha1.ResourceSelector{
							SecretRef: &v1alpha1.SecretSelector{
								Namespace: namespace,
								Name:      secret.Name,
								Key:       "SECRET1",
							},
						},
					},
					Password: v1alpha1.ResourceRef{
						SelectorType: v1alpha1.ResourceSelectorSecret,
						ResourceSelector: v1alpha1.ResourceSelector{
							SecretRef: &v1alpha1.SecretSelector{
								Namespace: namespace,
								Name:      secret.Name,
								Key:       "SECRET2",
							},
						},
					},
				},
			},
			expected: &auto.GitAuth{
				Username: "very secret",
				Password: "moar secret",
			},
		},
		{
			name: "GitAuthInvalidSecretReference",
			gitAuth: &v1alpha1.GitAuthConfig{
				PersonalAccessToken: &v1alpha1.ResourceRef{
					SelectorType: v1alpha1.ResourceSelectorSecret,
					ResourceSelector: v1alpha1.ResourceSelector{
						SecretRef: &v1alpha1.SecretSelector{
							Namespace: namespace,
							Name:      secret.Name,
							Key:       "MISSING",
						},
					},
				},
			},
			err: fmt.Errorf("resolving gitAuth personal access token: No key MISSING found in secret test/fake-secret"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := newReconcileStackSession(logger, v1alpha1.StackSpec{GitAuth: test.gitAuth}, client, namespace)
			gitAuth, err := session.SetupGitAuth()
			if test.err != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.expected, gitAuth)
		})
	}
}