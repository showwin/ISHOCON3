# ISHOCON3

<img src="https://user-images.githubusercontent.com/1732016/41643273-b4994c02-74a5-11e8-950d-3a1c1e54f44f.png" width="250px">

© [Chie Hayashi](https://www.facebook.com/hayashichie)


## ISHOCONとは
ISHOCONとは `Iikanjina SHOwwin CONtest` の略で、[ISUCON](http://isucon.net/)と同じように与えられたアプリケーションの高速化を競うコンテスト(?)です。

ISUCONは3人チームで取り組むことを基準に課題が作られていますが、ISHOCONは1人で8時間かけて解くことを基準に難易度を設定しています。

ISHOCON3 also supports English UI and comments within the code.

## 問題概要
今回のテーマは「新幹線チケット予約サイト」です。

私のように年末に新幹線で実家に帰省する勢は、1か月前に新幹線チケット争奪戦があるわけですが、座席予約のレースコンディションとか意外と大変なのではと思い題材にしてみました。
![](https://raw.githubusercontent.com/showwin/ISHOCON3/main/doc/images/main_page.png)

## 問題詳細
* 競技向けドキュメント:
  * [アプリケーション仕様書](https://github.com/showwin/ISHOCON3/blob/main/docs/app_spec_ja.md) ([EN](https://github.com/showwin/ISHOCON3/blob/main/docs/app_spec_en.md))
  * [マニュアル](https://github.com/showwin/ISHOCON3/blob/main/docs/manual_ja.md) ([EN](https://github.com/showwin/ISHOCON3/blob/main/docs/manual_en.md))
* AMI: `WIP`
* インスタンスタイプ: `c7i.xlarge`
* 参考実装言語: Python, Ruby(by [@Daivasmara](https://github.com/Daivasmara))
* 推奨実施時間: 1人で8時間


## 社内ISUCON等のイベントで使用したい方
自由に使って頂いて構いません。

イベント実施後にブログを書いて [@showwin](https://twitter.com/showwin) まで連絡頂けたら嬉しいです！下の関連リンクに掲載いたします。

サーバーの準備には terraform を使うと便利です。詳しくは [terraform の README](https://github.com/showwin/ISHOCON3/blob/main/contest/terraform/README.md) を参照してください。


## 気軽に楽しみたい方

ISHOCON3は、気軽に楽しめるように、Dockerを使ってローカルで動かすことができます。
詳しくは [こちらのドキュメント](https://github.com/showwin/ISHOCON3/blob/main/docs/local_dev.md) を参照してください。


## ISHOCONシリーズ
* [ISHOCON1](https://github.com/showwin/ISHOCON1)
* [ISHOCON2](https://github.com/showwin/ISHOCON2)
* [ISHOCON3](https://github.com/showwin/ISHOCON3)
