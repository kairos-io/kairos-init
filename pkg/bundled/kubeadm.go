package bundled

const KubeadmKubeUpgrade = `#!/bin/bash

exec   > >(tee -ia /var/log/kube-upgrade.log)
exec  2> >(tee -ia /var/log/kube-upgrade.log >& 2)
exec 19>> /var/log/kube-upgrade.log

set -x

NODE_ROLE=$1

root_path=$2
PROXY_CONFIGURED=$3
proxy_http=$4
proxy_https=$5
proxy_no=$6

export PATH="$PATH:$root_path/usr/bin"
export PATH="$PATH:$root_path/usr/local/bin"

if [ -n "$proxy_no" ]; then
  export NO_PROXY=$proxy_no
  export no_proxy=$proxy_no
fi

if [ -n "$proxy_http" ]; then
  export HTTP_PROXY=$proxy_http
  export http_proxy=$proxy_http
fi

if [ -n "$proxy_https" ]; then
  export https_proxy=$proxy_https
  export HTTPS_PROXY=$proxy_https
fi

CURRENT_NODE_NAME=$(cat /etc/hostname)

export KUBECONFIG=/etc/kubernetes/admin.conf

get_current_upgrading_node_name() {
  kubectl get configmap upgrade-lock -n kube-system --kubeconfig /etc/kubernetes/admin.conf -o jsonpath="{['data']['node']}"
}

delete_lock_config_map() {
  # Delete the configmap lock once the upgrade completes
  if [ "$NODE_ROLE" != "worker" ]
  then
    kubectl --kubeconfig /etc/kubernetes/admin.conf delete configmap upgrade-lock -n kube-system
  fi
}

restart_containerd() {
  if systemctl cat spectro-containerd >/dev/null 2<&1; then
    systemctl restart spectro-containerd
  fi

  if systemctl cat containerd >/dev/null 2<&1; then
    systemctl restart containerd
  fi
}

upgrade_kubelet() {
  echo "upgrading kubelet"
  systemctl stop kubelet
  cp /opt/kubeadm/bin/kubelet "$root_path"/usr/local/bin/kubelet
  systemctl daemon-reload && systemctl restart kubelet
  restart_containerd
  echo "kubelet upgraded"
}

apply_new_kubeadm_config() {
  kubectl get cm kubeadm-config -n kube-system -o jsonpath="{['data']['ClusterConfiguration']}" --kubeconfig /etc/kubernetes/admin.conf > "$root_path"/opt/kubeadm/existing-cluster-config.yaml
  kubeadm init phase upload-config kubeadm --config "$root_path"/opt/kubeadm/cluster-config.yaml
}

revert_kubeadm_config() {
  kubeadm init phase upload-config kubeadm --config "$root_path"/opt/kubeadm/existing-cluster-config.yaml
}

run_upgrade() {
    echo "running upgrade process on $NODE_ROLE"

    old_version=$(cat "$root_path"/opt/sentinel_kubeadmversion)
    echo "found last deployed version $old_version"

    current_version=$(kubeadm version -o short)
    echo "found current deployed version $current_version"

    # Check if the current kubeadm version is equal to the stored kubeadm version
    # If yes, do nothing
    if [ "$current_version" = "$old_version" ]
    then
      echo "node is on latest version"
      exit 0
    fi

    # Proceed to do upgrade operation

    # Try to create an empty configmap in default namespace which will act as a lock, until it succeeds.
    # Once a node creates a configmap, other nodes will remain at this step until the first node deletes the configmap when upgrade completes.
    if [ "$NODE_ROLE" != "worker" ]
    then
      until kubectl --kubeconfig /etc/kubernetes/admin.conf create configmap upgrade-lock -n kube-system --from-literal=node="${CURRENT_NODE_NAME}" > /dev/null
      do
        upgrade_node=$(get_current_upgrading_node_name)
        if [ "$upgrade_node" = "$CURRENT_NODE_NAME" ]; then
          echo "resuming upgrade"
          break
        fi
        echo "failed to create configmap for upgrade lock, upgrading is going on the node ${upgrade_node}, retrying in 60 sec"
        sleep 60
      done
    fi

    # Upgrade loop, runs until both stored and current is same
    until [ "$current_version" = "$old_version" ]
    do
        # worker node will always run 'upgrade node'
        # control plane will also run 'upgrade node' except one node will run 'upgrade apply' based on who acquires lock
        upgrade_command="kubeadm upgrade node"
        if [ "$PROXY_CONFIGURED" = true ]; then
          up=("kubeadm upgrade node")
          upgrade_command="${up[*]}"
        fi

        if [ "$NODE_ROLE" != "worker" ]
        then
            # The current api version is stored in kubeadm-config configmap
            # This is being used to check whether the current cp node will run 'upgrade apply' or not
            master_api_version=$(kubectl --kubeconfig /etc/kubernetes/admin.conf get cm kubeadm-config -n kube-system -o yaml | grep -m 1 kubernetesVersion | tr -s " " | cut -d' ' -f 3)
            if [ "$master_api_version" = "" ]; then
              echo "master api version empty, retrying in 60 seconds"
              sleep 60
              continue
            fi

            if [ "$master_api_version" = "$old_version" ]
            then
                apply_new_kubeadm_config
                upgrade_command="kubeadm upgrade apply -y $current_version"
                if [ "$PROXY_CONFIGURED" = true ]; then
                  up=("kubeadm upgrade apply -y ${current_version}")
                  upgrade_command="${up[*]}"
                fi
            fi
        fi
        echo "upgrading node from $old_version to $current_version using command: $upgrade_command"

        if sudo -E bash -c "$upgrade_command"
        then
            # Update current client version in the version file
            echo "$current_version" > "$root_path"/opt/sentinel_kubeadmversion
            old_version=$current_version

            delete_lock_config_map
            echo "upgrade success"
        else
            echo "upgrade failed"
            if echo "$upgrade_command" | grep -q "apply"; then
              echo "reverting kubeadm config"
              revert_kubeadm_config
            fi
            echo "retrying in 60 seconds"
            sleep 60
        fi
    done
    upgrade_kubelet
}

run_upgrade`

const KubeadmKubeReconfigure = `#!/bin/bash

set -x
trap 'echo -n $(date)' DEBUG

exec   > >(tee -ia /var/log/kube-reconfigure.log)
exec  2> >(tee -ia /var/log/kube-reconfigure.log >& 2)

info() {
    echo "[INFO] " "$@"
}

node_role=$1
certs_sans_revision=$2
kubelet_envs=$3
root_path=$4
custom_node_ip=$5
proxy_http=$6
proxy_https=$7
proxy_no=$8

export PATH="$PATH:$root_path/usr/bin"
export PATH="$PATH:$root_path/usr/local/bin"

certs_sans_revision_path="$root_path/opt/kubeadm/.kubeadm_certs_sans_revision"

if [ -n "$proxy_no" ]; then
  export NO_PROXY=$proxy_no
  export no_proxy=$proxy_no
fi

if [ -n "$proxy_http" ]; then
  export HTTP_PROXY=$proxy_http
  export http_proxy=$proxy_http
fi

if [ -n "$proxy_https" ]; then
  export https_proxy=$proxy_https
  export HTTPS_PROXY=$proxy_https
fi

export KUBECONFIG=/etc/kubernetes/admin.conf

regenerate_kube_components_manifests() {
  sudo -E bash -c "kubeadm init phase control-plane apiserver --config $root_path/opt/kubeadm/cluster-config.yaml"
  sudo -E bash -c "kubeadm init phase control-plane controller-manager --config $root_path/opt/kubeadm/cluster-config.yaml"
  sudo -E bash -c "kubeadm init phase control-plane scheduler --config $root_path/opt/kubeadm/cluster-config.yaml"

  kubeadm init phase upload-config kubeadm --config "$root_path"/opt/kubeadm/cluster-config.yaml

  info "regenerated kube components manifest"
}

regenerate_apiserver_certs_sans() {
  if [ ! -f "$certs_sans_revision_path" ]; then
    echo "$certs_sans_revision" > "$certs_sans_revision_path"
    return
  fi

  current_revision=$(cat "$certs_sans_revision_path")

  if [ "$certs_sans_revision" = "$current_revision" ]; then
    info "no change in certs sans revision"
    return
  fi

  rm /etc/kubernetes/pki/apiserver.{crt,key}
  info "regenerated removed existing apiserver certs"

  kubeadm init phase certs apiserver --config "$root_path"/opt/kubeadm/cluster-config.yaml
  info "regenerated apiserver certs"

  crictl pods 2>/dev/null | grep kube-apiserver | cut -d' ' -f1 | xargs -I %s sh -c '{ crictl stopp %s; crictl rmp %s; }' 2>/dev/null
  info "deleted existing apiserver pod"

  kubeadm init phase upload-config kubeadm --config "$root_path"/opt/kubeadm/cluster-config.yaml

  restart_kubelet
}

regenerate_kubelet_envs() {
  echo "$kubelet_envs" > /var/lib/kubelet/kubeadm-flags.env

  if [ "$node_role" != "worker" ];
  then
    mv /etc/kubernetes/kubelet.conf /etc/kubernetes/kubelet.conf.bak
    if [[ -n "$custom_node_ip" ]]; then
        kubeadm init phase kubeconfig kubelet --apiserver-advertise-address "$custom_node_ip"
    else
        kubeadm init phase kubeconfig kubelet
    fi
  fi

  systemctl restart kubelet
}

regenerate_kubelet_config() {
  PATCHES=$(awk '/patches:/{getline; print $2}' "$root_path"/opt/kubeadm/kubeadm.yaml)
  if [ "${PATCHES}" = "" ]; then
    kubeadm upgrade node phase kubelet-config
  else
    kubeadm upgrade node phase kubelet-config --patches $PATCHES
  fi
}

upload_kubelet_config() {
  kubeadm init phase upload-config kubelet --config "$root_path"/opt/kubeadm/kubelet-config.yaml
}

restart_kubelet() {
  systemctl restart kubelet
}

regenerate_etcd_manifests() {
  until kubectl --kubeconfig=/etc/kubernetes/admin.conf get cs > /dev/null
  do
    info "generating etcd manifests, cluster not accessible, retrying after 60 sec"
    sleep 60
    continue
  done
  kubeadm init phase etcd local --config "$root_path"/opt/kubeadm/cluster-config.yaml
  info "regenerated etcd manifest"
  sleep 60
}

update_file_permissions() {
  chmod 600 /var/lib/kubelet/config.yaml
  chmod 600 /etc/systemd/system/kubelet.service

  if [ -f /etc/kubernetes/pki/ca.crt ]; then
    chmod 600 /etc/kubernetes/pki/ca.crt
  fi

  if [ -f /etc/kubernetes/proxy.conf ]; then
    chown root:root /etc/kubernetes/proxy.conf
    chmod 600 /etc/kubernetes/proxy.conf
  fi
}

if [ "$node_role" != "worker" ];
then
  regenerate_kube_components_manifests
  regenerate_apiserver_certs_sans
  regenerate_etcd_manifests
  upload_kubelet_config
fi
regenerate_kubelet_config
regenerate_kubelet_envs
update_file_permissions
restart_kubelet
`

const KubeadmKubeReset = `#!/bin/bash

set -x
trap 'echo -n $(date)' DEBUG

if [ -f /etc/spectro/environment ]; then
  . /etc/spectro/environment
fi

export PATH="$PATH:$STYLUS_ROOT/usr/bin"
export PATH="$PATH:$STYLUS_ROOT/usr/local/bin"

if [ -S /run/spectro/containerd/containerd.sock ]; then
    kubeadm reset -f --cri-socket unix:///run/spectro/containerd/containerd.sock --cleanup-tmp-dir
else
    kubeadm reset -f --cleanup-tmp-dir
fi

iptables -F && iptables -t nat -F && iptables -t mangle -F && iptables -X
rm -rf /etc/kubernetes/etcd
rm -rf /etc/kubernetes/manifests
rm -rf /etc/kubernetes/pki
rm -rf /etc/containerd/config.toml
systemctl stop kubelet
if systemctl cat spectro-containerd >/dev/null 2<&1; then
  systemctl stop spectro-containerd
fi

if systemctl cat containerd >/dev/null 2<&1; then
  systemctl stop containerd
fi

umount -l /var/lib/kubelet
rm -rf /var/lib/kubelet && rm -rf ${STYLUS_ROOT}/var/lib/kubelet
rm -f $STYLUS_ROOT/usr/local/bin/kubelet
umount -l /var/lib/spectro/containerd
rm -rf /var/lib/spectro/containerd && rm -rf ${STYLUS_ROOT}/var/lib/spectro/containerd
umount -l /opt/bin
rm -rf /opt/bin && rm -rf ${STYLUS_ROOT}/opt/bin
umount -l /opt/cni/bin
rm -rf /opt/cni && rm -rf ${STYLUS_ROOT}/opt/cni
umount -l /etc/kubernetes
rm -rf /etc/kubernetes && rm -rf ${STYLUS_ROOT}/etc/kubernetes

rm -rf ${STYLUS_ROOT}/opt/kubeadm
rm -rf ${STYLUS_ROOT}/opt/containerd
rm -rf ${STYLUS_ROOT}/opt/*init
rm -rf ${STYLUS_ROOT}/opt/*join
rm -rf ${STYLUS_ROOT}/opt/kube-images
rm -rf ${STYLUS_ROOT}/opt/sentinel_kubeadmversion

rm -rf ${STYLUS_ROOT}/etc/systemd/system/spectro-kubelet.slice
rm -rf ${STYLUS_ROOT}/etc/systemd/system/spectro-containerd.slice
rm -rf ${STYLUS_ROOT}/etc/systemd/system/kubelet.service
rm -rf ${STYLUS_ROOT}/etc/systemd/system/containerd.service 2> /dev/null
rm -rf ${STYLUS_ROOT}/etc/systemd/system/spectro-containerd.service 2> /dev/null

rm -rf /var/log/kube*.log
rm -rf /var/log/apiserver
rm -rf /var/log/pods`

const KubeadmKubePreInit = `#!/bin/bash

exec   > >(tee -ia /var/log/kube-pre-init.log)
exec  2> >(tee -ia /var/log/kube-pre-init.log >& 2)
exec 19>> /var/log/kube-pre-init.log

export BASH_XTRACEFD="19"
set -x

root_path=$1

export PATH="$PATH:$root_path/usr/bin"
export PATH="$PATH:$root_path/usr/local/bin"

sysctl --system
modprobe overlay
modprobe br_netfilter
systemctl daemon-reload

if [ -f "$root_path"/opt/spectrocloud/kubeadm/bin/kubelet ]; then
  cp "$root_path"/opt/spectrocloud/kubeadm/bin/kubelet "$root_path"/usr/local/bin/kubelet
  systemctl daemon-reload
  systemctl enable kubelet && systemctl restart kubelet
  rm -rf "$root_path"/opt/spectrocloud/kubeadm/bin/kubelet
fi

if [ ! -f "$root_path"/usr/local/bin/kubelet ]; then
  mkdir -p "$root_path"/usr/local/bin
  cp "$root_path"/opt/kubeadm/bin/kubelet "$root_path"/usr/local/bin/kubelet
  systemctl enable kubelet && systemctl start kubelet
fi

if systemctl cat spectro-containerd >/dev/null 2<&1; then
  systemctl enable spectro-containerd && systemctl restart spectro-containerd
fi

if systemctl cat containerd >/dev/null 2<&1; then
  systemctl enable containerd && systemctl restart containerd
fi

if [ ! -f "$root_path"/opt/sentinel_kubeadmversion ]; then
  kubeadm version -o short > "$root_path"/opt/sentinel_kubeadmversion
fi`

const KubeadmKubePostInit = `#!/bin/bash

exec   > >(tee -ia /var/log/kube-post-init.log)
exec  2> >(tee -ia /var/log/kube-post-init.log >& 2)
exec 19>> /var/log/kube-post-init.log

export BASH_XTRACEFD="19"
set -x

root_path=$1

export KUBECONFIG=/etc/kubernetes/admin.conf
export PATH="$PATH:$root_path/usr/bin"
export PATH="$PATH:$root_path/usr/local/bin"

while true;
do
  secret=$(kubectl get secrets kubeadm-certs -n kube-system -o jsonpath="{['metadata']['ownerReferences'][0]['name']}")
  if [ "$secret" != "" ];
  then
    kubectl get secrets -n kube-system "${secret}" -o yaml | kubectl apply set-last-applied --create-annotation=true -f -
    kubectl get secrets -n kube-system "${secret}" -o yaml | sed '/^\( *\)expiration.*/d' | kubectl apply -f -
    echo "updated kubeadm-certs expiration"
    break
  else
    echo "failed to get kubeadm-certs ownerReferences, trying in 30 sec"
    sleep 30
  fi
done`

const KubeadmKubeJoin = `#!/bin/bash

exec   > >(tee -ia /var/log/kube-join.log)
exec  2> >(tee -ia /var/log/kube-join.log >& 2)
exec 19>> /var/log/kube-join.log

export BASH_XTRACEFD="19"
set -ex

NODE_ROLE=$1

root_path=$2
PROXY_CONFIGURED=$3
proxy_http=$4
proxy_https=$5
proxy_no=$6

export PATH="$PATH:$root_path/usr/bin"
export PATH="$PATH:$root_path/usr/local/bin"

KUBE_VIP_LOC="/etc/kubernetes/manifests/kube-vip.yaml"

restart_containerd() {
  if systemctl cat spectro-containerd >/dev/null 2<&1; then
    systemctl restart spectro-containerd
  fi

  if systemctl cat containerd >/dev/null 2<&1; then
    systemctl restart containerd
  fi
}

do_kubeadm_reset() {
  if [ -S /run/spectro/containerd/containerd.sock ]; then
    kubeadm reset -f --cri-socket unix:///run/spectro/containerd/containerd.sock --cleanup-tmp-dir
  else
    kubeadm reset -f --cleanup-tmp-dir
  fi
  iptables -F && iptables -t nat -F && iptables -t mangle -F && iptables -X && rm -rf /etc/kubernetes/etcd /etc/kubernetes/manifests /etc/kubernetes/pki
  rm -rf "$root_path"/etc/cni/net.d
  if [ -f /run/systemd/system/etc-cni-net.d.mount ]; then
    mkdir -p "$root_path"/etc/cni/net.d
    systemctl restart etc-cni-net.d.mount
  fi
  systemctl daemon-reload
  restart_containerd
}

backup_kube_vip_manifest_if_present() {
  if [ -f "$KUBE_VIP_LOC" ] && [ "$NODE_ROLE" != "worker" ]; then
    cp $KUBE_VIP_LOC "$root_path"/opt/kubeadm/kube-vip.yaml
  fi
}

restore_kube_vip_manifest_after_reset() {
  if [ -f "$root_path/opt/kubeadm/kube-vip.yaml" ] && [ "$NODE_ROLE" != "worker" ]; then
    mkdir -p "$root_path"/etc/kubernetes/manifests
    cp "$root_path"/opt/kubeadm/kube-vip.yaml $KUBE_VIP_LOC
  fi
}

if [ "$PROXY_CONFIGURED" = true ]; then
  until HTTP_PROXY=$proxy_http http_proxy=$proxy_http HTTPS_PROXY=$proxy_https https_proxy=$proxy_https NO_PROXY=$proxy_no no_proxy=$proxy_no kubeadm join --config "$root_path"/opt/kubeadm/kubeadm.yaml --ignore-preflight-errors=DirAvailable--etc-kubernetes-manifests -v=5 > /dev/null
  do
    backup_kube_vip_manifest_if_present
    echo "failed to apply kubeadm join, will retry in 10s";
    do_kubeadm_reset
    echo "retrying in 10s"
    sleep 10;
    restore_kube_vip_manifest_after_reset
  done;
else
  until kubeadm join --config "$root_path"/opt/kubeadm/kubeadm.yaml --ignore-preflight-errors=DirAvailable--etc-kubernetes-manifests -v=5 > /dev/null
  do
   backup_kube_vip_manifest_if_present
   echo "failed to apply kubeadm join, will retry in 10s";
   do_kubeadm_reset
   echo "retrying in 10s"
   sleep 10;
   restore_kube_vip_manifest_after_reset
  done;
fi`

const KubeadmKubeInit = `#!/bin/bash

exec   > >(tee -ia /var/log/kube-init.log)
exec  2> >(tee -ia /var/log/kube-init.log >& 2)
exec 19>> /var/log/kube-init.log

export BASH_XTRACEFD="19"
set -ex

root_path=$1
PROXY_CONFIGURED=$2
proxy_http=$3
proxy_https=$4
proxy_no=$5
KUBE_VIP_LOC="/etc/kubernetes/manifests/kube-vip.yaml"

export PATH="$PATH:$root_path/usr/bin"
export PATH="$PATH:$root_path/usr/local/bin"

restart_containerd() {
  if systemctl cat spectro-containerd >/dev/null 2<&1; then
    systemctl restart spectro-containerd
  fi

  if systemctl cat containerd >/dev/null 2<&1; then
    systemctl restart containerd
  fi
}

do_kubeadm_reset() {
  if [ -S /run/spectro/containerd/containerd.sock ]; then
    kubeadm reset -f --cri-socket unix:///run/spectro/containerd/containerd.sock --cleanup-tmp-dir
  else
    kubeadm reset -f --cleanup-tmp-dir
  fi

  iptables -F && iptables -t nat -F && iptables -t mangle -F && iptables -X && rm -rf /etc/kubernetes/etcd /etc/kubernetes/manifests /etc/kubernetes/pki
  rm -rf "$root_path"/etc/cni/net.d
  if [ -f /run/systemd/system/etc-cni-net.d.mount ]; then
    mkdir -p "$root_path"/etc/cni/net.d
    systemctl restart etc-cni-net.d.mount
  fi
  systemctl daemon-reload
  restart_containerd
}

backup_kube_vip_manifest_if_present() {
  if [ -f "$KUBE_VIP_LOC" ]; then
    cp $KUBE_VIP_LOC "$root_path"/opt/kubeadm/kube-vip.yaml
  fi
}

restore_kube_vip_manifest_after_reset() {
  if [ -f "$root_path/opt/kubeadm/kube-vip.yaml" ]; then
      mkdir -p "$root_path"/etc/kubernetes/manifests
      cp "$root_path"/opt/kubeadm/kube-vip.yaml $KUBE_VIP_LOC
  fi
}

if [ "$PROXY_CONFIGURED" = true ]; then
  until HTTP_PROXY=$proxy_http http_proxy=$proxy_http HTTPS_PROXY=$proxy_https https_proxy=$proxy_https NO_PROXY=$proxy_no no_proxy=$proxy_no kubeadm init --config "$root_path"/opt/kubeadm/kubeadm.yaml --upload-certs --ignore-preflight-errors=NumCPU --ignore-preflight-errors=Mem --ignore-preflight-errors=DirAvailable--etc-kubernetes-manifests -v=5 > /dev/null
  do
    backup_kube_vip_manifest_if_present
    echo "failed to apply kubeadm init, applying reset";
    do_kubeadm_reset
    echo "retrying in 10s"
    sleep 10;
    restore_kube_vip_manifest_after_reset
  done;
else
  until kubeadm init --config "$root_path"/opt/kubeadm/kubeadm.yaml --upload-certs --ignore-preflight-errors=NumCPU --ignore-preflight-errors=Mem --ignore-preflight-errors=DirAvailable--etc-kubernetes-manifests -v=5 > /dev/null
  do
    backup_kube_vip_manifest_if_present
    echo "failed to apply kubeadm init, applying reset";
    do_kubeadm_reset
    echo "retrying in 10s"
    sleep 10;
    restore_kube_vip_manifest_after_reset
  done;
fi`

const KubeadmKubeImagesLoad = `#!/bin/bash

# Kubeadm Load Config Images
#
# This script will download all kubeadm config images for a specific kubeadm/k8s version using crane command.
#
# Usage:
#   $0 $kubeadm_version

set -ex

KUBE_VERSION=$1

ARCH=$(uname -m)
OS=$(uname)
ARCHIVE_NAME=go-containerregistry_"${OS}"_"${ARCH}".tar.gz
TEMP_DIR=/opt/kubeadm-temp
IMAGES_DIR=/opt/kube-images
IMAGE_FILE=images.list

# create temp dir
mkdir -p $TEMP_DIR && mkdir -p $IMAGES_DIR
cd $TEMP_DIR || exit

verify_downloader() {
  cmd="$(command -v "${1}")"
  if [ -z "${cmd}" ]; then
      return 1
  fi
  if [ ! -x "${cmd}" ]; then
      return 1
  fi

  DOWNLOADER=${cmd}
  return 0
}

download_crane() {
  verify_downloader curl || verify_downloader wget
  case ${DOWNLOADER} in
  *curl)
    curl -L -o "${ARCHIVE_NAME}" https://github.com/google/go-containerregistry/releases/download/v0.13.0/"${ARCHIVE_NAME}"
    ;;
  *wget)
    wget https://github.com/google/go-containerregistry/releases/download/v0.13.0/"${ARCHIVE_NAME}"
    ;;
  *)
    echo "curl or wget not found"
    exit 1
  esac
}

download_crane
tar -xvf "${ARCHIVE_NAME}" crane

# Put all kubeadm image into a file
kubeadm config images list --kubernetes-version "${KUBE_VERSION}" > $IMAGE_FILE

# create tar
while read -r image; do
  IFS="/" read -r -a im <<< $image
  image_name_with_version="${im[-1]}"

  IFS=":" read -r -a ima <<< $image_name_with_version
  name="${ima[0]}"
  version="${ima[1]}"

  ./crane pull "$image" ${IMAGES_DIR}/"${name}"-"${version}".tar
done < $IMAGE_FILE

rm -rf ${TEMP_DIR}`

const KubeadmKubeImport = `#!/bin/bash -x

CONTENT_PATH=$1

# find all tar files recursively
for tarfile in $(find $CONTENT_PATH -name "*.tar" -type f)
do
  # try to import the tar file into containerd up to ten times
  for i in {1..10}
  do
    if [ -S /run/spectro/containerd/containerd.sock ]; then
      /opt/bin/ctr -n k8s.io --address /run/spectro/containerd/containerd.sock image import "$tarfile" --all-platforms
    else
      /opt/bin/ctr -n k8s.io image import "$tarfile" --all-platforms
    fi
    if [ $? -eq 0 ]; then
      echo "Import successful: $tarfile (attempt $i)"
      break
    else
      if [ $i -eq 10 ]; then
        echo "Import failed: $tarfile (attempt $i)"
      fi
    fi
  done
done`
