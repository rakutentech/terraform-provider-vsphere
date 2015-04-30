terraform-provider-vsphere
==========================

[![GitHub release](http://img.shields.io/github/release/rakutentech/terraform-provider-vsphere.svg?style=flat-square)][release]
[![Wercker](http://img.shields.io/wercker/ci/54e197683e14329223213f6e.svg?style=flat-square)][wercker]
[![Go Documentation](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)][godocs]

[release]: https://github.com/rakutentech/terraform-provider-vsphere/releases 
[wercker]: https://app.wercker.com/#applications/54e197683e14329223213f6e
[license]: https://github.com/tcnksm/ghr/blob/master/LICENSE
[godocs]: http://godoc.org/github.com/rakutentech/terraform-provider-vsphere

Terraform Custom Provider for VMware vSphere

## Description

This project is a terraform custom provider for VMware vSphere. This is work in
progress. This current version only supports creation and deletion of virtual
machine with VM template.

## Requirement

* [hashicorp/terraform](https://github.com/hashicorp/terraform)
* [vmware/govmomi](https://github.com/vmware/govmomi)

### VM template specification

In this use case, please don't include network adapters in VM template. When 
deploying a new virtual machine, this provider adds new network adapters to the
new virtual machine. Supported network adapter type in the current version is 
VMXNET 3 only.

## Usage

### Provider Configuration

#### `vsphere`

```
provider "vsphere" {
    user = "${var.vsphere_user}"
    password = "${var.vsphere_password}"
    vcenter_server = "${var.vsphere_vcenter}"
}
```

##### Argument Reference

The following arguments are supported.

* `user` - (Required) This is the user name to access to vCenter server.
* `password` - (Required) This is the password to access to vCenter server.
* `vcenter_server` - (Required) This is a target vCenter server, such as "vcenter.my.domain.com"

### Resource Configuration

#### `vsphere_virtual_machine`

```
resource "vsphere_virtual_machine" "default" {
    name = "VM name"
    datacenter = "Datacenter name"
    cluster = "Cluster name"
    datastore = "Datastore name"
    template = "templates/centos-6.6-x86_64"
    vcpu = 2
    memory = 4096
    gateway = "Gateway ip address"
    network_interface {
        label = "Network label name"
        ip_address = "IP address"
        subnet_mask = "Subnet mask"
    }
}
```

##### Argument Reference

The following arguments are supported.

* `name` - (Required) Hostname of the virtual machine
* `template` - (Required) VM template name
* `vcpu` - (Required) A number of vCPUs
* `memory` - (Required) Memory size in MB.
* `network_interface` - (Required) Network configuration.
* `datacenter` - (Optional) Datacenter name
* `cluster` - (Optional) Cluster name, a cluster is a group of hosts.
* `resource_pool` - (Optional) Resource pool name.
* `datastore` - (Optional) Datastore name
* `additional_disk` - (Required) Additional hard disk configuration.
* `gateway` - (Optional) Gateway IP address. If you use the static IP address, it's required.
* `time_zone` - (Optional) Time zone configuration. By default, it's "Etc/UTC".
* `domain` - (Optional) Domain configuration. By default, it's "vsphere.local".
* `dns_suffix` - (Optional) List of DNS suffix. By default, it's `["vsphere.local"]`.
* `dns_server` - (Optional) List of DNS server. By default, it's `["8.8.8.8", "8.8.4.4"]`.

Each `network_interface` supports the following:

* `label` - (Required) Network label name.
* `ip_address` - (Optional) IP address. DHCP configuration in default. If you use the static IP address, it's required.
* `subnet_mask` - (Optional) Subnet mask. If you use the static IP address, it's required.

Each `additional_disk` supports the following:

* `size` - (Required) Size of additional hard disk in gigabytes. 
* `iops` - (Optional) IOPS number. By default, it's unlimited.


For example

```
resource "vsphere_virtual_machine" "default" {
    name = "foo-1"
    template = "centos-6.6-x86_64"    # Template name
    vcpu = 2
    memory = 4096
    network_interface {
        label = "VM Network"
    }
}
```


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

