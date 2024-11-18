dependencies {
    api(project(":common-turbo:common-turbo-client"))
    api("com.tencent.bk.devops.ci.auth:api-auth:${Versions.ciVersion}") {
        isTransitive = false
    }
    api("com.tencent.bk.devops.ci.common:common-auth-api:${Versions.ciVersion}"){
        isTransitive = false
    }
}
