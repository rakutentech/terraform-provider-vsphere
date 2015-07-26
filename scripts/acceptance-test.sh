#!/bin/sh

NAME="terraform-provider-vsphere"
SRC_PATH="/go/src/github.com/rakutentech/terraform-provider-vsphere"

cat << EOT > ./Dockerfile
FROM golang:1.4.1-cross
ENV http_proxy $http_proxy
ENV https_proxy $https_proxy
EOT

cat << 'EOT' >> ./Dockerfile
ENV TF_ACC 1
RUN go get -u github.com/mitchellh/gox
RUN go get -u github.com/hashicorp/terraform
RUN cd $GOPATH/src/github.com/hashicorp/terraform/ && make updatedeps && make dev
RUN go get -u github.com/vmware/govmomi
EOT

#sudo docker build --no-cache -t ${NAME} .
sudo docker build -t ${NAME} .
sudo docker run --rm -v "$(pwd)":${SRC_PATH}/${NAME} \
    -w ${SRC_PATH}/${NAME}/vsphere \
    -e "VSPHERE_DATACENTER=${VSPHERE_DATACENTER}" \
    -e "VSPHERE_CLUSTER=${VSPHERE_CLUSTER}" \
    -e "VSPHERE_DATASTORE=${VSPHERE_DATASTORE}" \
    -e "VSPHERE_TEMPLATE=${VSPHERE_TEMPLATE}" \
    -e "VSPHERE_NETWORK_LABEL=${VSPHERE_NETWORK_LABEL}" \
    -e "VSPHERE_USER=${VSPHERE_USER}" \
    -e "VSPHERE_PASSWORD=${VSPHERE_PASSWORD}" \
    -e "VSPHERE_VCENTER=${VSPHERE_VCENTER}" \
    -e "VSPHERE_NETWORK_GATEWAY=${VSPHERE_NETWORK_GATEWAY}" \
    -e "VSPHERE_NETWORK_IP_ADDRESS=${VSPHERE_NETWORK_IP_ADDRESS}" \
    -e "VSPHERE_NETWORK_LABEL_DHCP=${VSPHERE_NETWORK_LABEL_DHCP}" \
    -e "VSPHERE_VM_PASSWORD=${VSPHERE_VM_PASSWORD}" \
    ${NAME} go test -v
