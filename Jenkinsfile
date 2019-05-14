pipeline {
  agent {
    docker {
      image 'kubespheredev/porter-infra:0.0.1'
      args '-v $HOME:/root -v /var/run/docker.sock:/var/run/docker.sock  -v /usr/bin/docker:/usr/bin/docker -v /tmp:/tmp'
    }
  }
  environment {
    tag = sh(
      script: 'git rev-parse --short HEAD',
      returnStdout: true
    ).trim()
    ACCESS_KEY_ID     = credentials('jenkins-qc-secret-key-id')
    SECRET_ACCESS_KEY = credentials('jenkins-qc-secret-access-key')
    IMG =  "magicsong/cloud-manager:$tag"
    API_OWNER = "usr-MRiIUq7M"
  }
  stages {
    stage('Building Manager'){
      steps{
        sh """
            CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags "-w" -o bin/manager ./cmd/main.go
            docker build -t $IMG  -f deploy/Dockerfile bin/
            echo "Push images"
            docker push $IMG
        """
      }
    }
    stage('Test') {
      steps {
        sh """            
            ./hack/e2e.sh -s
          """
      }
    }
  }
  post {
        failure {
          echo "Detect failure .Save logs "
          archiveArtifacts artifacts: 'cloud_controller.log'
        }
        always {
            echo 'Clean images'
            archiveArtifacts artifacts: 'test/*.yaml', fingerprint: true
            sh """
              docker rmi $IMG  || echo "No image to remove"
            """
        }
    }
}