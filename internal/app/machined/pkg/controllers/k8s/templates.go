// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package k8s

import "github.com/siderolabs/talos/pkg/flannel"

// kube-apiserver configuration:

var kubeSystemEncryptionConfigTemplate = []byte(`apiVersion: v1
kind: EncryptionConfig
resources:
- resources:
  - secrets
  providers:
  {{if .Root.SecretboxEncryptionSecret}}
  - secretbox:
      keys:
      - name: key2
        secret: {{ .Root.SecretboxEncryptionSecret }}
  {{end}}
  {{if .Root.AESCBCEncryptionSecret}}
  - aescbc:
      keys:
      - name: key1
        secret: {{ .Root.AESCBCEncryptionSecret }}
  {{end}}
  - identity: {}
`)

// manifests injected into kube-apiserver

var kubeletBootstrappingToken = []byte(`apiVersion: v1
kind: Secret
metadata:
  name: bootstrap-token-{{ .Secrets.BootstrapTokenID }}
  namespace: kube-system
type: bootstrap.kubernetes.io/token
stringData:
  token-id: "{{ .Secrets.BootstrapTokenID }}"
  token-secret: "{{ .Secrets.BootstrapTokenSecret }}"
  usage-bootstrap-authentication: "true"

  # Extra groups to authenticate the token as. Must start with "system:bootstrappers:"
  auth-extra-groups: system:bootstrappers:nodes
`)

// csrNodeBootstrapTemplate lets bootstrapping tokens and nodes request CSRs.
var csrNodeBootstrapTemplate = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: system-bootstrap-node-bootstrapper
subjects:
- kind: Group
  name: system:bootstrappers:nodes
  apiGroup: rbac.authorization.k8s.io
- kind: Group
  name: system:nodes
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: system:node-bootstrapper
  apiGroup: rbac.authorization.k8s.io
`)

// csrApproverRoleBindingTemplate instructs the csrapprover controller to
// automatically approve CSRs made by bootstrapping tokens for client
// credentials.
//
// This binding should be removed to disable CSR auto-approval.
var csrApproverRoleBindingTemplate = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: system-bootstrap-approve-node-client-csr
subjects:
- kind: Group
  name: system:bootstrappers:nodes
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: system:certificates.k8s.io:certificatesigningrequests:nodeclient
  apiGroup: rbac.authorization.k8s.io
`)

// csrRenewalRoleBindingTemplate instructs the csrapprover controller to
// automatically approve all CSRs made by nodes to renew their client
// certificates.
//
// This binding should be altered in the future to hold a list of node
// names instead of targeting `system:nodes` so we can revoke individual
// node's ability to renew its certs.
var csrRenewalRoleBindingTemplate = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: system-bootstrap-node-renewal
subjects:
- kind: Group
  name: system:nodes
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: system:certificates.k8s.io:certificatesigningrequests:selfnodeclient
  apiGroup: rbac.authorization.k8s.io
`)

var kubeProxyTemplate = []byte(`apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: kube-proxy
  namespace: kube-system
  labels:
    tier: node
    k8s-app: kube-proxy
spec:
  selector:
    matchLabels:
      tier: node
      k8s-app: kube-proxy
  template:
    metadata:
      labels:
        tier: node
        k8s-app: kube-proxy
    spec:
      containers:
      - name: kube-proxy
        image: {{ .ProxyImage }}
        command:
        - /usr/local/bin/kube-proxy
        {{- range $arg := .ProxyArgs }}
        - {{ $arg | json }}
        {{- end }}
        env:
          - name: NODE_NAME
            valueFrom:
              fieldRef:
                fieldPath: spec.nodeName
          - name: POD_IP
            valueFrom:
              fieldRef:
                fieldPath: status.podIP
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /lib/modules
          name: lib-modules
          readOnly: true
        - mountPath: /etc/ssl/certs
          name: ssl-certs-host
          readOnly: true
        - name: kubeconfig
          mountPath: /etc/kubernetes
          readOnly: true
      hostNetwork: true
      priorityClassName: system-cluster-critical
      serviceAccountName: kube-proxy
      tolerations:
      - effect: NoSchedule
        operator: Exists
      - effect: NoExecute
        operator: Exists
      volumes:
      - name: lib-modules
        hostPath:
          path: /lib/modules
      - name: ssl-certs-host
        hostPath:
          path: /etc/ssl/certs
      - name: kubeconfig
        configMap:
          name: kubeconfig-in-cluster
  updateStrategy:
    rollingUpdate:
      maxUnavailable: 1
    type: RollingUpdate
---
apiVersion: v1
kind: ServiceAccount
metadata:
  namespace: kube-system
  name: kube-proxy
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kube-proxy
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:node-proxier # Automatically created system role.
subjects:
- kind: ServiceAccount
  name: kube-proxy
  namespace: kube-system
`)

// kubeConfigInCluster instructs clients to use their service account token,
// but unlike an in-cluster client doesn't rely on the `KUBERNETES_SERVICE_PORT`
// and `KUBERNETES_PORT` to determine the API servers address.
//
// This kubeconfig is used by bootstrapping pods that might not have access to
// these env vars, such as kube-proxy, which sets up the API server endpoint
// (chicken and egg), and the checkpointer, which needs to run as a static pod
// even if the API server isn't available.
var kubeConfigInClusterTemplate = []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: kubeconfig-in-cluster
  namespace: kube-system
data:
  kubeconfig: |
    apiVersion: v1
    clusters:
    - name: local
      cluster:
        server: {{ .Server }}
        certificate-authority: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    users:
    - name: service-account
      user:
        # Use service account token
        tokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    contexts:
    - context:
        cluster: local
        user: service-account
`)

var coreDNSTemplate = []byte(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: coredns
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: system:coredns
  labels:
    kubernetes.io/bootstrapping: rbac-defaults
  annotations:
    rbac.authorization.kubernetes.io/autoupdate: "true"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:coredns
subjects:
  - kind: ServiceAccount
    name: coredns
    namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: system:coredns
  labels:
    kubernetes.io/bootstrapping: rbac-defaults
rules:
  - apiGroups: [""]
    resources:
      - endpoints
      - services
      - pods
      - namespaces
    verbs:
      - list
      - watch
  - apiGroups: ["discovery.k8s.io"]
    resources:
      - endpointslices
    verbs:
      - list
      - watch
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    .:53 {
        errors
        health {
            lameduck 5s
        }
        ready
        log . {
            class error
        }
        prometheus :9153

        kubernetes {{ .ClusterDomain }} in-addr.arpa ip6.arpa {
            pods insecure
            fallthrough in-addr.arpa ip6.arpa
        }
        forward . /etc/resolv.conf
        cache 30
        loop
        reload
        loadbalance
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: coredns
  namespace: kube-system
  labels:
    k8s-app: kube-dns
    kubernetes.io/name: "CoreDNS"
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
  selector:
    matchLabels:
      k8s-app: kube-dns
  template:
    metadata:
      labels:
        k8s-app: kube-dns
    spec:
      nodeSelector:
        kubernetes.io/os: linux
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: k8s-app
                  operator: In
                  values:
                  - kube-dns
              topologyKey: kubernetes.io/hostname
      serviceAccountName: coredns
      priorityClassName: system-cluster-critical
      tolerations:
        - key: node-role.kubernetes.io/control-plane
          operator: Exists
          effect: NoSchedule
        - key: node.cloudprovider.kubernetes.io/uninitialized
          operator: Exists
          effect: NoSchedule
      containers:
        - name: coredns
          image: {{ .CoreDNSImage }}
          imagePullPolicy: IfNotPresent
          resources:
            limits:
              memory: 170Mi
            requests:
              cpu: 100m
              memory: 70Mi
          env:
            - name: GOMEMLIMIT
              value: "161MiB"
          args: [ "-conf", "/etc/coredns/Corefile" ]
          volumeMounts:
            - name: config-volume
              mountPath: /etc/coredns
              readOnly: true
          ports:
            - name: dns
              protocol: UDP
              containerPort: 53
            - name: dns-tcp
              protocol: TCP
              containerPort: 53
            - name: metrics
              protocol: TCP
              containerPort: 9153
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
              scheme: HTTP
            initialDelaySeconds: 60
            timeoutSeconds: 5
            successThreshold: 1
            failureThreshold: 5
          readinessProbe:
            httpGet:
              path: /ready
              port: 8181
              scheme: HTTP
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              add:
              - NET_BIND_SERVICE
              drop:
              - ALL
            readOnlyRootFilesystem: true
      dnsPolicy: Default
      volumes:
        - name: config-volume
          configMap:
            name: coredns
            items:
            - key: Corefile
              path: Corefile
`)

var coreDNSSvcTemplate = []byte(`apiVersion: v1
kind: Service
metadata:
  name: kube-dns
  namespace: kube-system
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9153"
  labels:
    k8s-app: kube-dns
    kubernetes.io/cluster-service: "true"
    kubernetes.io/name: "CoreDNS"
spec:
  selector:
    k8s-app: kube-dns
  clusterIP: {{ or .DNSServiceIP .DNSServiceIPv6 }}
  clusterIPs:
  {{- if .DNSServiceIP }}
    - {{ .DNSServiceIP }}
  {{- end }}
  {{- if .DNSServiceIPv6 }}
    - {{ .DNSServiceIPv6 }}
  {{- end }}
  ipFamilies:
  {{- if .DNSServiceIP }}
    - IPv4
  {{- end }}
  {{- if .DNSServiceIPv6 }}
    - IPv6
  {{- end }}
  {{- if and .DNSServiceIP .DNSServiceIPv6 }}
  ipFamilyPolicy: RequireDualStack
  {{- else }}
  ipFamilyPolicy: SingleStack
  {{- end }}
  ports:
    - name: dns
      port: 53
      protocol: UDP
    - name: dns-tcp
      port: 53
      protocol: TCP
    - name: metrics
      port: 9153
      protocol: TCP
`)

// podSecurityPolicy is the default PSP.
var podSecurityPolicy = []byte(`kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: psp:privileged
rules:
- apiGroups: ['policy']
  resources: ['podsecuritypolicies']
  verbs:     ['use']
  resourceNames:
  - privileged
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: psp:privileged
roleRef:
  kind: ClusterRole
  name: psp:privileged
  apiGroup: rbac.authorization.k8s.io
subjects:
# Authorize all service accounts in a namespace:
- kind: Group
  apiGroup: rbac.authorization.k8s.io
  name: system:serviceaccounts
# Authorize all authenticated users in a namespace:
- kind: Group
  apiGroup: rbac.authorization.k8s.io
  name: system:authenticated
---
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: privileged
  annotations:
    seccomp.security.alpha.kubernetes.io/allowedProfileNames: '*'
spec:
  fsGroup:
    rule: RunAsAny
  privileged: true
  runAsUser:
    rule: RunAsAny
  seLinux:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  volumes:
  - '*'
  allowedCapabilities:
  - '*'
  hostPID: true
  hostIPC: true
  hostNetwork: true
  hostPorts:
  - min: 1
    max: 65536
`)

// talosAPIService is the service to access Talos API from Kubernetes.
// Service exposes the Endpoints which are managed by controllers.
var talosAPIService = []byte(`apiVersion: v1
kind: Service
metadata:
  labels:
    component: apid
    provider: talos
  name: {{ .KubernetesTalosAPIServiceName }}
  namespace: {{ .KubernetesTalosAPIServiceNamespace }}
spec:
  ports:
  - name: apid
    port: {{ .ApidPort }}
    protocol: TCP
    targetPort: {{ .ApidPort }}
`)

var flannelTemplate = flannel.Template

// talosServiceAccountCRDTemplate is the template of the CRD which
// allows injecting Talos with credentials into the Kubernetes cluster.
var talosServiceAccountCRDTemplate = []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: {{ .TalosServiceAccount.ResourcePlural }}.{{ .TalosServiceAccount.Group }}
spec:
  conversion:
    strategy: None
  group: {{ .TalosServiceAccount.Group }}
  names:
    kind: {{ .TalosServiceAccount.Kind }}
    listKind: {{ .TalosServiceAccount.Kind }}List
    plural: {{ .TalosServiceAccount.ResourcePlural }}
    singular: {{ .TalosServiceAccount.ResourceSingular }}
    shortNames:
      - {{ .TalosServiceAccount.ShortName }}
  scope: Namespaced
  versions:
    - name: {{ .TalosServiceAccount.Version }}
      schema:
        openAPIV3Schema:
          properties:
            spec:
              type: object
              properties:
                roles:
                  type: array
                  items:
                    type: string
            status:
              type: object
              properties:
                failureReason:
                  type: string
          type: object
      served: true
      storage: true
`)

var talosHostDNSSvcTemplate = []byte(`apiVersion: v1
kind: Service
metadata:
  name: host-dns
  namespace: kube-system
spec:
  clusterIP: {{ .ServiceHostDNSAddress }}
  ports:
  - name: dns
    port: 53
    protocol: UDP
    targetPort: 53
  - name: dns-tcp
    port: 53
    protocol: TCP
    targetPort: 53
  type: ClusterIP
---
apiVersion: v1
kind: Endpoints
metadata:
  name: host-dns
  namespace: kube-system
subsets:
  - addresses:
    - ip: {{ .ServiceHostDNSAddress }}
    ports:
    - name: dns
      port: 53
      protocol: UDP
    - name: dns-tcp
      port: 53
      protocol: TCP
`)
