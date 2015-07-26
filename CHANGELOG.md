CHANGELOG
=========

## 0.3.1 (July 24, 2015)

Bugfixes:

  - Change default network adapter type and disk type for creating new Virtual Machine ([**@tkak**](https://github.com/tkak))
  - Add task.Wait function to fix a failure in destroying VM ([**@tkak**](https://github.com/tkak))
  - Move default DNS suffixes value and default DNS servers value to global scope ([**@tkak**](https://github.com/tkak))
  - Use GetOk function ([**@tkak**](https://github.com/tkak))
  - Fix findDatastore bug ([**@tkak**](https://github.com/tkak))


## 0.3.0 (May 28, 2015)

Improvements:

  - Support additional disk feature and create virtual machine feature with `disk` argument ([**@tkak**](https://github.com/tkak))


## 0.2.0 (April 28, 2015)

Improvements:

  - Update govmomi package ([**@tkak**](https://github.com/tkak))
  - Remove network device name from `vsphere_virtual_machine` resource ([**@tkak**](https://github.com/tkak))
  - Support `time_zone` parameter ([**@tkak**](https://github.com/tkak))

Bugfixes:

  - Add ForceNew option and modify for passing InternalValidate ([**@tkak**](https://github.com/tkak))
  - Fix wrong binary name of this terraform plugin ([**@tkak**](https://github.com/tkak))
  - VM Template in child folders can be specified. #4 ([**@tkak**](https://github.com/tkak))


## 0.1.0 (February 24, 2015)

  - Initial release ([**@tkak**](https://github.com/tkak), supported by [**@tcnksm**](https://github.com/tcnksm))

