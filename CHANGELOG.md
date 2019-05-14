# Changelog
## [ 1.3.4 ] - 2019-05-13
### Added
- get userid automatically [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/55)
  
### Changed
- simplify `Makefile`
- remove certs mounts [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/56)
- recording logs when e2e-test failure
 
## [ 1.3.3 ] - 2019-05-04
### Added
- support UDP [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/54)
### Fixed
- fixed that `https` port becomes `http` in qingcloud console [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/54)
- fixed securityGroup is not deleted in cloud

## [ 1.3.2 ] - 2019-05-06
### Added
- refact ut framework and bring code-coverage up to 65% [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/52)

### Changed
- remove https protocol [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/52)

### Fixed
- fixed a lot of tiny bugs thanks to new ut framework

## [ 1.3.1 ] - 2019-05-04
### Fixed
- fixed that lb cannot get an ip if there is no avaliable eips in `auto` mode [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/51)

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