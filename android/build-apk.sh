#!/bin/bash
# 构建 rtp Android APK（无 Gradle，直接用 build-tools 命令行打包）
# 需要：JDK 17、Android SDK（platforms;android-34、build-tools;34.0.0）
set -e
export JAVA_HOME=${JAVA_HOME:-/usr/lib/jvm/java-17-openjdk-amd64}
export ANDROID_HOME=${ANDROID_HOME:-/opt/android-sdk}
BT=$ANDROID_HOME/build-tools/34.0.0
PLATFORM=$ANDROID_HOME/platforms/android-34/android.jar
APP_DIR=$(cd "$(dirname "$0")/app" && pwd)
OUT=${1:-$(dirname "$0")/rtp-android.apk}
WORK=/tmp/rtp-apk-build
VERSION_NAME=${VERSION_NAME:-1.1.0}

rm -rf $WORK
mkdir -p $WORK/compiled $WORK/dex $WORK/gen $WORK/apk $WORK/tmp

echo "==> 1/5 编译资源 (aapt2)"
$BT/aapt2 compile --dir $APP_DIR/res -o $WORK/compiled.zip
$BT/aapt2 link \
  -I $PLATFORM \
  --manifest $APP_DIR/AndroidManifest.xml \
  -R $WORK/compiled.zip \
  --java $WORK/gen \
  --min-sdk-version 24 \
  --target-sdk-version 34 \
  --auto-add-overlay \
  -o $WORK/apk/base.apk

echo "==> 2/5 编译 Java"
find $APP_DIR/src $WORK/gen -name '*.java' > $WORK/sources.txt
$JAVA_HOME/bin/javac \
  -source 11 -target 11 \
  -classpath $PLATFORM \
  -nowarn \
  -d $WORK/classes \
  @$WORK/sources.txt
test -d $WORK/classes || { echo "javac 未产出 class，请检查编译错误"; exit 1; }

echo "==> 3/5 dex (d8)"
find $WORK/classes -name '*.class' > $WORK/classes.txt
$BT/d8 --release --min-api 24 --lib $PLATFORM --output $WORK/dex @$WORK/classes.txt

echo "==> 4/5 打包 classes.dex"
cp $WORK/apk/base.apk $WORK/apk/unsigned.apk
python3 - "$WORK/apk/unsigned.apk" "$WORK/dex/classes.dex" << 'PYEOF'
import sys, zipfile
apk, dex = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(apk, 'a', zipfile.ZIP_DEFLATED) as z:
    z.write(dex, 'classes.dex')
with zipfile.ZipFile(apk) as z:
    assert 'classes.dex' in z.namelist(), 'classes.dex 打包失败'
print('classes.dex 已注入 APK')
PYEOF

echo "==> 5/5 签名"
$BT/zipalign -f 4 $WORK/apk/unsigned.apk $WORK/apk/aligned.apk

# 生成调试签名（首次自动生成）
KEYSTORE=$WORK/tmp/rtp.keystore
if [ ! -f "$KEYSTORE" ]; then
  $JAVA_HOME/bin/keytool -genkeypair -v \
    -keystore $KEYSTORE -storepass rtp123456 -keypass rtp123456 \
    -alias rtp -keyalg RSA -keysize 2048 -validity 10950 \
    -dname "CN=rtp, OU=rtp, O=rtp, L=SH, ST=SH, C=CN" > /dev/null 2>&1
fi

$BT/apksigner sign \
  --ks $KEYSTORE --ks-pass pass:rtp123456 --key-pass pass:rtp123456 \
  --ks-key-alias rtp \
  --out $OUT \
  $WORK/apk/aligned.apk

$BT/apksigner verify $OUT && echo "签名校验通过"
$BT/aapt2 dump badging $OUT | head -3
ls -la $OUT
echo "构建完成: $OUT"