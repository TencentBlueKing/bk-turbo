dependencies {
    api("org.quartz-scheduler:quartz")
    api("org.quartz-scheduler:quartz-jobs")
    api("io.fluidsonic.mirror:quartz-mongodb:2.2.0-rc2")
    api("org.springframework.boot:spring-boot-starter")
    api("org.springframework:spring-context-support")
    api("org.springframework:spring-tx")
    api(project(":common-turbo:common-turbo-api"))
}
