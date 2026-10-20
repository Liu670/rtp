package com.rtp.app;

import android.app.Activity;
import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.CheckBox;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONObject;

import java.util.List;

/**
 * MainActivity —— 配置服务端地址 / 令牌 / 名称，一键连接；展示连接状态、隧道列表与实时流量。
 */
public class MainActivity extends Activity {

    private static final Handler UI = new Handler(Looper.getMainLooper());

    private EditText edServer, edToken, edName;
    private CheckBox cbAuto;
    private Button btnToggle;
    private TextView tvState, tvTraffic, tvTunnels, tvLog;
    private LinearLayout tunnelBox;
    private volatile boolean connected = false;

    private SharedPreferences sp;

    // ---- 供 Service 回调（Service 与 Activity 可能不同生命周期，用静态引用） ----
    static MainActivity instance;

    static void postState(String state, String detail) {
        final MainActivity a = instance;
        if (a == null) return;
        UI.post(() -> a.onState(state, detail));
    }

    static void postLog(String line) {
        final MainActivity a = instance;
        if (a == null) return;
        UI.post(() -> {
            a.tvLog.append(line + "\n");
        });
    }

    static void postTraffic(long up, long down) {
        final MainActivity a = instance;
        if (a == null) return;
        UI.post(() -> a.tvTraffic.setText("↑ " + fmt(up) + "    ↓ " + fmt(down)));
    }

    static void postTunnels(List<RtpClient.Tunnel> tunnels) {
        final MainActivity a = instance;
        if (a == null) return;
        UI.post(() -> a.renderTunnels(tunnels));
    }

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        instance = this;
        sp = getSharedPreferences("rtp", Context.MODE_PRIVATE);

        ScrollView scroll = new ScrollView(this);
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(28, 40, 28, 40);
        scroll.addView(root);
        setContentView(scroll);
        getWindow().setStatusBarColor(Color.parseColor("#4F46E5"));

        // ---------- 品牌 ----------
        TextView logo = new TextView(this);
        logo.setText("rtp");
        logo.setTextSize(40);
        logo.setTextColor(Color.parseColor("#4F46E5"));
        logo.setGravity(Gravity.CENTER);
        logo.setPadding(0, 0, 0, 4);
        root.addView(logo);

        TextView sub = new TextView(this);
        sub.setText("内网穿透 · 手机客户端");
        sub.setTextSize(14);
        sub.setTextColor(Color.parseColor("#64748B"));
        sub.setGravity(Gravity.CENTER);
        sub.setPadding(0, 0, 0, 28);
        root.addView(sub);

        // ---------- 表单 ----------
        edServer = input(root, "rtp 服务端地址", "例如 43.142.62.24:7000");
        edToken = input(root, "客户端令牌", "控制台「客户端」页复制");
        edToken.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_VISIBLE_PASSWORD);
        edName = input(root, "本机名称", "例如 my-phone");

        cbAuto = new CheckBox(this);
        cbAuto.setText("开机自动启动连接");
        cbAuto.setTextColor(Color.parseColor("#334155"));
        root.addView(cbAuto);

        btnToggle = new Button(this);
        btnToggle.setText("连接");
        btnToggle.setTextSize(16);
        btnToggle.setOnClickListener(v -> onToggle());
        LinearLayout.LayoutParams blp = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        blp.topMargin = 24;
        root.addView(btnToggle, blp);

        // ---------- 状态卡片 ----------
        LinearLayout card = card(root, "连接状态");
        tvState = line(card, "未连接", 15, "#334155");
        tvTraffic = line(card, "↑ 0 B    ↓ 0 B", 14, "#64748B");
        tvTunnels = line(card, "隧道：0 条", 14, "#64748B");
        tvLog = line(card, "", 12, "#94A3B8");
        tvLog.setPadding(0, 12, 0, 0);

        LinearLayout tunnelWrap = card(root, "生效中的隧道");
        tunnelBox = new LinearLayout(this);
        tunnelBox.setOrientation(LinearLayout.VERTICAL);
        tunnelWrap.addView(tunnelBox);

        // ---------- 说明 ----------
        TextView help = line(root, "使用说明\n\n"
                + "1. 在 rtp 控制台「客户端」页新建一个客户端，复制它的令牌；\n"
                + "2. 填入上面的「服务端地址」与「客户端令牌」，点「连接」；\n"
                + "3. 回到控制台为该客户端新建隧道，例如本地 8080 → 公网 18080；\n"
                + "4. 隧道建立后，手机上的 8080 服务即可从公网访问。", 13, "#64748B");
        help.setLineSpacing(4, 1.15f);
        help.setPadding(0, 24, 0, 0);

        // 恢复配置
        edServer.setText(sp.getString("server", ""));
        edToken.setText(sp.getString("token", ""));
        edName.setText(sp.getString("name", "my-phone"));
        cbAuto.setChecked(sp.getBoolean("auto", false));
    }

    @Override
    protected void onDestroy() {
        if (instance == this) instance = null;
        super.onDestroy();
    }

    private void onToggle() {
        String server = edServer.getText().toString().trim();
        String token = edToken.getText().toString().trim();
        String name = edName.getText().toString().trim();

        if (connected) {
            RtpService.stop(this);
            connected = false;
            btnToggle.setText("连接");
            tvState.setText("已断开");
            return;
        }
        if (server.isEmpty() || token.isEmpty()) {
            toast("请填写服务端地址与客户端令牌");
            return;
        }
        String[] hp = RtpClient.parseServer(server);
        int port;
        try {
            port = Integer.parseInt(hp[1]);
        } catch (Exception e) {
            toast("端口格式不正确");
            return;
        }
        if (name.isEmpty()) name = "my-phone";

        sp.edit().putString("server", server).putString("token", token)
                .putString("name", name).putBoolean("auto", cbAuto.isChecked()).apply();

        RtpService.start(this, hp[0], port, token, name);
        connected = true;
        btnToggle.setText("断开连接");
        tvLog.setText("");
    }

    private void onState(String state, String detail) {
        tvState.setText(detail);
        String color = "#334155";
        if ("online".equals(state)) color = "#16A34A";
        else if ("offline".equals(state)) color = "#DC2626";
        else if ("connecting".equals(state)) color = "#D97706";
        tvState.setTextColor(Color.parseColor(color));
    }

    private void renderTunnels(List<RtpClient.Tunnel> tunnels) {
        tunnelBox.removeAllViews();
        if (tunnels == null || tunnels.isEmpty()) {
            TextView t = line(tunnelBox, "暂无隧道，请在控制台为该客户端新建隧道", 13, "#94A3B8");
            t.setPadding(0, 8, 0, 8);
            return;
        }
        for (RtpClient.Tunnel tun : tunnels) {
            LinearLayout row = new LinearLayout(this);
            row.setOrientation(LinearLayout.VERTICAL);
            row.setPadding(0, 12, 0, 12);

            TextView title = new TextView(this);
            title.setText(protoTag(tun.proto) + "  " + tun.name);
            title.setTextSize(14);
            title.setTextColor(Color.parseColor("#0F172A"));
            row.addView(title);

            TextView detail = new TextView(this);
            StringBuilder sb = new StringBuilder();
            sb.append("本地 ").append(tun.localHost).append(":").append(tun.localPort);
            if (tun.publicPort > 0) sb.append("   →  公网 :").append(tun.publicPort);
            if (!tun.p2pMode.isEmpty()) sb.append("   [").append(tun.p2pMode).append("]");
            detail.setText(sb.toString());
            detail.setTextSize(12);
            detail.setTextColor(Color.parseColor("#64748B"));
            row.addView(detail);

            tunnelBox.addView(row);
            View line = new View(this);
            line.setBackgroundColor(Color.parseColor("#E2E8F0"));
            line.setLayoutParams(new LinearLayout.LayoutParams(
                    LinearLayout.LayoutParams.MATCH_PARENT, 1));
            tunnelBox.addView(line);
        }
        tvTunnels.setText("隧道：" + tunnels.size() + " 条");
    }

    private String protoTag(String proto) {
        switch (proto == null ? "" : proto) {
            case "tcp": return "🔵 TCP";
            case "udp": return "🟣 UDP";
            case "http": return "🟢 HTTP";
            case "https": return "🔒 HTTPS";
            case "stcp": return "🟠 STCP";
            default: return "⚪ " + proto;
        }
    }

    // ---------- 简易 UI 构件 ----------

    private EditText input(LinearLayout root, String label, String hint) {
        TextView lb = line(root, label, 13, "#475569");
        lb.setPadding(0, 16, 0, 6);
        EditText et = new EditText(this);
        et.setHint(hint);
        et.setHintTextColor(Color.parseColor("#94A3B8"));
        et.setTextColor(Color.parseColor("#0F172A"));
        et.setTextSize(15);
        et.setInputType(InputType.TYPE_CLASS_TEXT);
        et.setPadding(14, 14, 14, 14);
        et.setBackground(round(Color.parseColor("#F1F5F9"), 10));
        root.addView(et);
        return et;
    }

    private LinearLayout card(LinearLayout root, String title) {
        LinearLayout c = new LinearLayout(this);
        c.setOrientation(LinearLayout.VERTICAL);
        c.setPadding(20, 16, 20, 16);
        c.setBackground(round(Color.parseColor("#F8FAFC"), 14));
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        lp.topMargin = 24;
        root.addView(c, lp);
        TextView t = new TextView(this);
        t.setText(title);
        t.setTextSize(13);
        t.setTextColor(Color.parseColor("#94A3B8"));
        c.addView(t);
        return c;
    }

    private TextView line(LinearLayout parent, String text, int size, String color) {
        TextView t = new TextView(this);
        t.setText(text);
        t.setTextSize(size);
        t.setTextColor(Color.parseColor(color));
        t.setPadding(0, 6, 0, 0);
        parent.addView(t);
        return t;
    }

    private android.graphics.drawable.GradientDrawable round(int color, int radiusDp) {
        android.graphics.drawable.GradientDrawable gd = new android.graphics.drawable.GradientDrawable();
        gd.setColor(color);
        gd.setCornerRadius(radiusDp * getResources().getDisplayMetrics().density);
        return gd;
    }

    private void toast(String msg) {
        Toast.makeText(this, msg, Toast.LENGTH_SHORT).show();
    }

    private static String fmt(long b) {
        if (b < 1024) return b + " B";
        if (b < 1024 * 1024) return String.format("%.1f KB", b / 1024.0);
        if (b < 1024L * 1024 * 1024) return String.format("%.1f MB", b / 1024.0 / 1024);
        return String.format("%.2f GB", b / 1024.0 / 1024 / 1024);
    }
}