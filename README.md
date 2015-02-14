terraform-provider-vsphere
==========================

Terraform Custom Provider for VMWare vSphere

## Description

This project is a terraform custom provider for VMWare vSphere. This is work in progress. 
This current version only supports creation and deletion of virtual machine with VM template.

## Requirement

* [hashicorp/terraform](https://github.com/hashicorp/terraform)
* [vmware/govmomi](https://github.com/vmware/govmomi)

## Usage

### Provider Configuration

```
provider "vsphere" {
    user = "${var.vsphere_user}"
    password = "${var.vsphere_password}"
    vcenter_server = "${var.vsphere_vcenter}"
}
```

#### Argument Reference

The following arguments are supported.

* `user` - (Required) This is the user name to access to vCenter server.
* `password` - (Required) This is the password to access to vCenter server.
* `vcenter_server` - (Required) This is a target vCenter server, such as "vcenter.my.domain.com"

### Resource Configuration

vsphere_virtual_machine

```
resource "vsphere_virtual_machine" "default" {
    name = "VM name"
    datacenter = "Datacenter name"
    cluster = "Cluster name"
    datastore = "Datastore name"
    template = "centos-6.6-x86_64"    # Template name
    vcpu = 2
    memory = 4096
    gateway = "Gateway ip address"
    network {
        device_name = "NIC name"      # e.g. eth0
        label = "Network label name"
        ip_address = "IP address"
        subnet_mask = "Subnet mask"
    }
}
```

#### Argument Reference

The following arguments are supported.

* `name` - (Required) Hostname of the virtual machine
* `template` - (Required) VM template name
* `datacenter` - (Optional) Datacenter name
* `cluster` - (Optional) Cluster name, a cluster is a group of hosts.
* `datastore` - (Optional) Datastore name
* `vcpu` - (Optional) A number of vCPUs
* `memory` - (Optional) Memory size in MB. By default the same as VM.
* `gateway` - (Optional) Gateway IP address.
* `domain` - (Optional) Domain configuration.
* `network_interface` - (Optional) Network configuration.

Each `network_interface` supports the following:

* `device_name` - (Required) Network interface device name
* `label` - (Required) Network label name
* `ip_address` - (Required) IP address
* `subnet_mask` - (Required) Subnet mask


## Contribution

1. Fork it
2. Create your feature branch (`git checkout -b my-new-feature`)
3. Commit your changes (`git commit -am 'Add some feature'`)
4. Push to the branch (`git push origin my-new-feature`)
5. Create new Pull Request


## Licence

[Mozilla Public License, version 2.0](https://github.com/rakutentech/terraform-provider-vsphere/blob/master/LICENSE)

## Author

[tkak](https://github.com/tkak)

