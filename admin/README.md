## アプリケーションイメージ作成手順(メモ)
```
$ docker build -f Dockerfile_app . -t showwin/ishocon3_app:$version
$ docker login
$ docker push showwin/ishocon3_app:$version
```

## ベンチマーカーイメージ作成手順(メモ)
```
$ docker build -f Dockerfile_bench . -t showwin/ishocon3_bench:$version
$ docker login
$ docker push showwin/ishocon3_bench:$version
```
