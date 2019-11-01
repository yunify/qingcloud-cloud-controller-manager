# Changelog
## [ 1.4.2 ] - 2019-10-29
### Added
- internal lb support custom ip [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/89)
- support sharing internal lb [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/90)
- docs about internal lb [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/91)
### Fixed
- lb not deleted when its type changed from `LoadBalancer` to `NodePort`[@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/88)

## [ 1.4.1 ] - 2019-10-28
### Added
- support internal load balancer [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/85)
- support use users' lb [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/84)

## [ 1.4.0 ] - 2019-09-05
### Added
- support adding tags to resources [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/78)
### Changed
- ✨change gcfg to yaml [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/76)
- ✈️ refactor error handling [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/75)
- simplify deployment [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/78)

### Fixed
- 🚒 fix updates not working as expected [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/70)
- 🚒 fix listener not working when ports changing from "aabb" style to "aa" [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/71)
- 🚒 fix jenkinsfile [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/73)
- 🚒 add cleanup before/after e2e [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/80)

## [ 1.3.5 ] - 2019-08-07
### Fixed
- when changing ports in service, old listeners are not deleted [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/68)
- fix bug in auto mode [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/61)

### Added
- add cluster mode in instance [@magicsong](https://github.com/yunify/qingcloud-cloud-controller-manager/pull/64)

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