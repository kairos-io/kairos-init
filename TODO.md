
- support for anything other than ubuntu
- add packages for the rest of the distros
- add path for model support (i.e rpi4 has a different kernel than mainline)
- add path for variant (easy, variant just install k3s like we do on the dockerfile and thats it)
- add workarounds. Like the chmod of sudo for ubuntu we should have some kind of final workaround for all distros and versions, so things runs and clear up things that we need to fix for each distro
- add support for alpine especifically like the services are only using systemctl with no guards and there is no openrc plugin for yip