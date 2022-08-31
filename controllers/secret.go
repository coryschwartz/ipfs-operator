package controllers

import (
	"context"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1alpha1 "github.com/redhat-et/ipfs-operator/api/v1alpha1"
)

const (
	peerIDPrefix     = "peerID-"
	privateKeyPrefix = "privateKey-"
)

func (r *IpfsReconciler) secretConfig(
	m *clusterv1alpha1.Ipfs,
	sec *corev1.Secret,
	clusterSecret,
	bootstrapPrivateKey []byte,
) (controllerutil.MutateFn, string) {
	secName := "ipfs-cluster-" + m.Name

	expectedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secName,
			Namespace: m.Namespace,
		},
	}
	// find secret
	err := r.Get(context.Background(), client.ObjectKeyFromObject(expectedSecret), expectedSecret)
	if err != nil && !errors.IsNotFound(err) {
		return func() error {
			return err
		}, ""
	}
	// initialize the secret, if needed
	if err != nil && errors.IsNotFound(err) {
		// secret doesn't exist
		generateNewIdentities(expectedSecret, 0, m.Spec.Replicas)
		expectedSecret.Data = make(map[string][]byte)
		expectedSecret.Data["CLUSTER_SECRET"] = clusterSecret
		expectedSecret.Data["BOOTSTRAP_PEER_PRIV_KEY"] = bootstrapPrivateKey
	}

	// secret does exist,
	numIdentities := countIdentities(expectedSecret)
	if numIdentities != m.Spec.Replicas {
		// create more identities if needed, otherwise they will be reused
		// when scaling down and then up again
		if int(numIdentities) < int(m.Spec.Replicas) {
			// create more
			generateNewIdentities(expectedSecret, numIdentities, m.Spec.Replicas)
		}
	}

	expectedSecret.DeepCopyInto(sec)
	// FIXME: catch this error before we run the function being returned
	if err := ctrl.SetControllerReference(m, sec, r.Scheme); err != nil {
		return func() error { return err }, ""
	}
	return func() error {
		sec.Data = expectedSecret.Data
		return nil
	}, secName
}

// countIdentities Counts the amount of unique peer identities present in the secret.
func countIdentities(secret *corev1.Secret) int32 {
	var count int32 = 0
	for key := range secret.Data {
		if strings.Contains(key, peerIDPrefix) {
			count++
		}
	}
	return count
}

// generateNewIdentities Populates the secret data with new Peer IDs
// and private keys which are mapped based on the replica number.
func generateNewIdentities(secret *corev1.Secret, start, n int32) error {
	secret.StringData = make(map[string]string)
	for i := start; i < n; i++ {
		// generate new private key & peer id
		peerID, privKey, err := generateIdentity()
		if err != nil {
			return err
		}
		peerIDKey := peerIDPrefix + strconv.Itoa(int(i))
		secret.StringData[peerIDKey] = peerID.String()
		secretKey := privateKeyPrefix + strconv.Itoa(int(i))
		secret.StringData[secretKey] = privKey
	}
	return nil
}
