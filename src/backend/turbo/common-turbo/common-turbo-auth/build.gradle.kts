dependencies {
    api(project(":common-turbo:common-turbo-client"))
    api("com.tencent.bk.devops.ci.auth:api-auth") {
        isTransitive = false
    }
    api("com.tencent.bk.devops.ci.common:common-auth-api"){
        isTransitive = false
    }
}
