package vsphere

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
)

func TestAccVSphereVirtualMachine_Basic(t *testing.T) {
	var vm VirtualMachine
	name := os.Getenv("VSPHERE_VM_NAME")
	datacenter := os.Getenv("VSPHERE_DATACENTER")
	cluster := os.Getenv("VSPHERE_CLUSTER")
	datastore := os.Getenv("VSPHERE_DATASTORE")
	template := os.Getenv("VSPHERE_TEMPLATE")
	label := os.Getenv("VSPHERE_NETWORK_LABEL")

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckVSphereVirtualMachineDestroy,
		Steps: []resource.TestStep{
			resource.TestStep{
				Config: fmt.Sprintf(
					testAccCheckVSphereVirtualMachineConfig_basic,
					name,
					datacenter,
					cluster,
					datastore,
					template,
					label,
				),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVSphereVirtualMachineExists("vsphere_virtual_machine.foobar", &vm),
					//		testAccCheckVSphereVirtualMachineAttributes(&vm),
					resource.TestCheckResourceAttr(
						"vsphere_virtual_machine.foobar", "name", name),
				),
			},
		},
	})
}

func testAccCheckVSphereVirtualMachineDestroy(s *terraform.State) error {
	client := testAccProvider.Meta().(*govmomi.Client)
	finder := find.NewFinder(client, true)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vsphere_virtual_machine" {
			continue
		}

		dc, err := finder.Datacenter(rs.Primary.Attributes["datacenter"])
		if err != nil {
			return fmt.Errorf("error %s", err)
		}

		dcFolders, err := dc.Folders()
		if err != nil {
			return fmt.Errorf("error %s", err)
		}

		_, err = client.SearchIndex().FindChild(dcFolders.VmFolder, rs.Primary.Attributes["name"])
		if err == nil {
			return fmt.Errorf("Record still exists")
		}
	}

	return nil
}

func testAccCheckVSphereVirtualMachineExists(n string, vm *VirtualMachine) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		client := testAccProvider.Meta().(*govmomi.Client)
		finder := find.NewFinder(client, true)

		dc, err := finder.Datacenter(rs.Primary.Attributes["datacenter"])
		if err != nil {
			return fmt.Errorf("error %s", err)
		}

		dcFolders, err := dc.Folders()
		if err != nil {
			return fmt.Errorf("error %s", err)
		}

		_, err = client.SearchIndex().FindChild(dcFolders.VmFolder, rs.Primary.Attributes["name"])
		/*
			vmRef, err := client.SearchIndex().FindChild(dcFolders.VmFolder, rs.Primary.Attributes["name"])
			if err != nil {
				return fmt.Errorf("error %s", err)
			}

			found := govmomi.NewVirtualMachine(client, vmRef.Reference())
			fmt.Printf("%v", found)

			if found.Name != rs.Primary.ID {
				return fmt.Errorf("Instance not found")
			}
			*instance = *found
		*/

		*vm = VirtualMachine{
			Name: rs.Primary.ID,
		}

		return nil
	}
}

const testAccCheckVSphereVirtualMachineConfig_basic = `
resource "vsphere_virtual_machine" "foobar" {
    name = "%s"
    datacenter = "%s"
    cluster = "%s"
    datastore = "%s"
    template = "%s"
    vcpu = 2
    memory = 4096
    gateway = "192.168.0.254"
    network_interface {
        device_name = "eth0"
        label = "%s"
        ip_address = "192.168.0.10"
        subnet_mask = "255.255.255.0"
    }
}
`
