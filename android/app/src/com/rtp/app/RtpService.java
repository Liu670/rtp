package com.rtp.app;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.os.Build;
import android.os.IBinder;
import android.util.Base64;

/**
 * RtpService —— 前台服务，保证连接常驻（配合开机自启选项）。
 */
public class RtpService extends Service {

    public static final String EXTRA_HOST = "host";
    public static final String EXTRA_PORT = "port";
    public static final String EXTRA_TOKEN = "token";
    public static final String EXTRA_NAME = "name";
    public static final String ACTION_STOP = "com.rtp.app.STOP";

    private RtpClient client;
    private static final String CH_ID = "rtp_conn";

    public static void start(Context ctx, String host, int port, String token, String name) {
        Intent i = new Intent(ctx, RtpService.class);
        i.putExtra(EXTRA_HOST, host);
        i.putExtra(EXTRA_PORT, port);
        i.putExtra(EXTRA_TOKEN, token);
        i.putExtra(EXTRA_NAME, name);
        ctx.startForegroundService(i);
    }

    public static void stop(Context ctx) {
        Intent i = new Intent(ctx, RtpService.class);
        i.setAction(ACTION_STOP);
        ctx.startService(i);
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && ACTION_STOP.equals(intent.getAction())) {
            if (client != null) client.stop();
            stopForeground(true);
            stopSelf();
            return START_NOT_STICKY;
        }

        String host = intent == null ? "" : intent.getStringExtra(EXTRA_HOST);
        int port = intent == null ? 7000 : intent.getIntExtra(EXTRA_PORT, 7000);
        String token = intent == null ? "" : intent.getStringExtra(EXTRA_TOKEN);
        String name = intent == null ? "android" : intent.getStringExtra(EXTRA_NAME);

        createChannel();
        startForeground(1, buildNotification("正在启动…"));

        if (client != null) client.stop();
        client = new RtpClient(host, port, token, name, new RtpClient.Listener() {
            @Override
            public void onState(String state, String detail) {
                MainActivity.postState(state, detail);
                if ("online".equals(state)) {
                    pushNotify("已连接 rtp 服务端");
                } else if ("offline".equals(state)) {
                    pushNotify("连接断开：" + detail);
                }
            }

            @Override
            public void onLog(String line) {
                MainActivity.postLog(line);
            }

            @Override
            public void onTunnels(java.util.List<RtpClient.Tunnel> tunnels) {
                // UDP 隧道需要客户端侧常驻中继连接
                MainActivity.postTunnels(tunnels);
            }

            @Override
            public void onTraffic(long up, long down) {
                MainActivity.postTraffic(up, down);
            }
        });
        client.start();
        return START_STICKY;
    }

    private void createChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            NotificationChannel ch = new NotificationChannel(CH_ID, "连接状态", NotificationManager.IMPORTANCE_LOW);
            NotificationManager nm = getSystemService(NotificationManager.class);
            if (nm != null) nm.createNotificationChannel(ch);
        }
    }

    private Notification buildNotification(String text) {
        Intent open = new Intent(this, MainActivity.class);
        open.setFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent pi = PendingIntent.getActivity(this, 0, open, PendingIntent.FLAG_IMMUTABLE);
        PendingIntent stopPi = PendingIntent.getService(this, 1,
                new Intent(this, RtpService.class).setAction(ACTION_STOP), PendingIntent.FLAG_IMMUTABLE);
        return new Notification.Builder(this, CH_ID)
                .setContentTitle("rtp 内网穿透")
                .setContentText(text)
                .setSmallIcon(android.R.drawable.stat_sys_upload_done)
                .setContentIntent(pi)
                .addAction(new Notification.Action.Builder(null, "断开", stopPi).build())
                .setOngoing(true)
                .build();
    }

    private void pushNotify(String text) {
        NotificationManager nm = getSystemService(NotificationManager.class);
        if (nm != null) nm.notify(1, buildNotification(text));
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    @Override
    public void onDestroy() {
        if (client != null) client.stop();
        super.onDestroy();
    }
}