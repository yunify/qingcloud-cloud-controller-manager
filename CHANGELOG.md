# Changelog

## [ 1.3.0 ] - 2019-04-30
### Added
-  add some ut [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/45)
-  support eip reuse [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/43)
-  support specifying prototol in lb [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/43)
-  support ip auto allocate [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/48)
-  use secret instead of host path when reading API key [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/47)

### Changed
- refine codes for making it easy to write unit tests
- add more e2e-tests

## [ 1.2.0 ] - 2019-04-16
### Added
- support kubernetes 1.14.1 [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/30)
- support non-instance-id hostname [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/35)
- add e2e-test [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/36)


### Changed
- use vgo [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/30)
-  reduce binary size by removing other cloudprovider [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/30)