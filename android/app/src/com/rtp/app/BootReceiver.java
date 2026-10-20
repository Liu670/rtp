package com.rtp.app;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

/** 开机自启：读取上次配置，若用户勾选了「开机自动启动」则恢复连接。 */
public class BootReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context ctx, Intent intent) {
        if (intent == null || intent.getAction() == null) return;
        if (!Intent.ACTION_BOOT_COMPLETED.equals(intent.getAction())) return;

        android.content.SharedPreferences sp = ctx.getSharedPreferences("rtp", Context.MODE_PRIVATE);
        if (!sp.getBoolean("auto", false)) return;

        String server = sp.getString("server", "");
        String token = sp.getString("token", "");
        String name = sp.getString("name", "my-phone");
        if (server.isEmpty() || token.isEmpty()) return;

        String[] hp = RtpClient.parseServer(server);
        int port;
        try {
            port = Integer.parseInt(hp[1]);
        } catch (Exception e) {
            return;
        }
        RtpService.start(ctx, hp[0], port, token, name);
    }
}