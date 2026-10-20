package com.rtp.app;

import android.util.Base64;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.io.OutputStreamWriter;
import java.io.Writer;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * RtpClient —— rtp 内网穿透客户端（Android 原生 Java 实现）。
 *
 * 与服务端 rtps 的行协议一致：每行为一个 JSON 对象 + '\n'。
 * 控制连接负责认证与规则同步，工作连接负责实际流量透传。
 */
public class RtpClient {

    public interface Listener {
        void onState(String state, String detail);
        void onLog(String line);
        void onTunnels(List<Tunnel> tunnels);
        void onTraffic(long up, long down);
    }

    /** 一条隧道规则（服务端下推）。 */
    public static class Tunnel {
        public String id = "", name = "", proto = "", localHost = "127.0.0.1";
        public int localPort, publicPort;
        public String p2pMode = "";
    }

    private final String serverHost;   // 仅主机名/IP
    private final String serverAddr;   // host:port
    private final String token;
    private final String name;
    private final Listener listener;

    private final ExecutorService pool = Executors.newCachedThreadPool();
    private final AtomicBoolean running = new AtomicBoolean(false);
    private final Map<String, Tunnel> tunnels = new HashMap<>();
    private final Map<String, Socket> udpRelays = new HashMap<>();

    private volatile String workPort = "0";
    private volatile long upBytes = 0;
    private volatile long downBytes = 0;
    private Thread trafficTimer;

    public RtpClient(String serverHost, int serverPort, String token, String name, Listener listener) {
        this.serverHost = serverHost;
        this.serverAddr = serverHost + ":" + serverPort;
        this.token = token;
        this.name = name;
        this.listener = listener;
    }

    public void start() {
        if (!running.compareAndSet(false, true)) return;
        pool.execute(() -> {
            while (running.get()) {
                try {
                    connectOnce();
                } catch (Exception e) {
                    if (running.get()) listener.onState("offline", "连接断开：" + e.getMessage());
                }
                if (running.get()) {
                    listener.onState("connecting", "3 秒后重连...");
                    sleep(3000);
                }
            }
        });
        trafficTimer = new Thread(() -> {
            while (running.get()) {
                listener.onTraffic(upBytes, downBytes);
                sleep(1000);
            }
        });
        trafficTimer.setDaemon(true);
        trafficTimer.start();
    }

    public void stop() {
        running.set(false);
        synchronized (udpRelays) {
            for (Socket s : udpRelays.values()) closeQuietly(s);
            udpRelays.clear();
        }
        pool.shutdownNow();
    }

    public List<Tunnel> tunnelSnapshot() {
        synchronized (tunnels) {
            return new ArrayList<>(tunnels.values());
        }
    }

    // ---------------- 控制连接 ----------------

    private void connectOnce() throws Exception {
        listener.onState("connecting", "正在连接 " + serverAddr);
        Socket sock = new Socket();
        sock.connect(new InetSocketAddress(serverHost, Integer.parseInt(serverAddr.split(":")[1])), 10000);
        sock.setTcpNoDelay(true);
        BufferedReader in = new BufferedReader(new InputStreamReader(sock.getInputStream(), StandardCharsets.UTF_8));
        Writer w = new OutputStreamWriter(sock.getOutputStream(), StandardCharsets.UTF_8);

        JSONObject auth = new JSONObject();
        auth.put("type", "auth");
        auth.put("token", token);
        auth.put("client_id", name);
        writeMsg(w, auth);

        JSONObject resp = readMsg(in);
        if (!resp.optBoolean("ok", false)) {
            sock.close();
            throw new Exception("认证失败：" + resp.optString("message", "未知原因"));
        }
        workPort = String.valueOf(resp.optInt("work_port", 0));
        listener.onState("online", "已连接，工作端口 " + workPort);
        listener.onLog("认证成功，等待服务端下发隧道规则");

        String line;
        while (running.get() && (line = in.readLine()) != null) {
            if (line.trim().isEmpty()) continue;
            JSONObject m;
            try {
                m = new JSONObject(line);
            } catch (Exception e) {
                continue;
            }
            handleCtrl(m);
        }
        sock.close();
    }

    private void handleCtrl(JSONObject m) throws Exception {
        String type = m.optString("type");
        switch (type) {
            case "sync":
            case "add_tunnel": {
                List<Tunnel> list = new ArrayList<>();
                if (type.equals("sync")) {
                    synchronized (tunnels) {
                        tunnels.clear();
                        JSONArray arr = m.optJSONArray("tunnels");
                        if (arr != null) {
                            for (int i = 0; i < arr.length(); i++) {
                                Tunnel t = parseTunnel(arr.getJSONObject(i));
                                if (t != null) tunnels.put(t.id, t);
                            }
                        }
                    }
                    list.addAll(tunnels.values());
                } else {
                    JSONObject to = m.optJSONObject("tunnel");
                    Tunnel t = parseTunnel(to);
                    if (t != null) {
                        synchronized (tunnels) {
                            tunnels.put(t.id, t);
                        }
                        list.addAll(tunnels.values());
                    }
                }
                listener.onTunnels(list);
                listener.onLog("隧道规则已同步：" + list.size() + " 条");
                break;
            }
            case "remove_tunnel": {
                String tid = m.optString("tunnel_id");
                synchronized (tunnels) {
                    tunnels.remove(tid);
                }
                synchronized (udpRelays) {
                    Socket s = udpRelays.remove(tid);
                    closeQuietly(s);
                }
                listener.onTunnels(tunnelSnapshot());
                break;
            }
            case "new_conn":
                pool.execute(() -> handleNewConn(m.optString("conn_id"), m.optString("tunnel_id")));
                break;
            case "ping": {
                JSONObject pong = new JSONObject();
                pong.put("type", "pong");
                // pong 走独立短连接由工作端处理，这里通过控制连接的写线程不可用，
                // 采用直接写 socket 缓冲的方式（服务端容忍并发小写入）。
                pongSocketWrite(pong);
                break;
            }
            default:
                break;
        }
    }

    private Socket ctrlSocket;

    private void pongSocketWrite(JSONObject pong) {
        // 服务端对 ping 的响应要求及时，用独立连接回一条最稳妥
        pool.execute(() -> {
            try (Socket s = new Socket()) {
                s.connect(new InetSocketAddress(serverHost, Integer.parseInt(serverAddr.split(":")[1])), 5000);
                Writer w = new OutputStreamWriter(s.getOutputStream(), StandardCharsets.UTF_8);
                writeMsg(w, pong);
                s.getInputStream().read();
            } catch (Exception ignored) {
            }
        });
    }

    private Tunnel parseTunnel(JSONObject o) {
        if (o == null) return null;
        Tunnel t = new Tunnel();
        t.id = o.optString("id");
        t.name = o.optString("name");
        t.proto = o.optString("proto");
        String lh = o.optString("local_host");
        t.localHost = lh.isEmpty() ? "127.0.0.1" : lh;
        t.localPort = o.optInt("local_port");
        t.publicPort = o.optInt("public_port");
        t.p2pMode = o.optString("p2p_mode", "");
        return t;
    }

    // ---------------- 工作连接：实际流量 ----------------

    private void handleNewConn(String connID, String tunnelID) {
        Tunnel t;
        synchronized (tunnels) {
            t = tunnels.get(tunnelID);
        }
        if (t == null) {
            listener.onLog("收到未知隧道的新连接，已忽略");
            return;
        }
        Socket local = null;
        Socket work = null;
        try {
            local = new Socket();
            local.connect(new InetSocketAddress(t.localHost, t.localPort), 5000);
            local.setTcpNoDelay(true);

            work = new Socket();
            work.connect(new InetSocketAddress(serverHost, Integer.parseInt(workPort)), 10000);
            work.setTcpNoDelay(true);
            Writer ww = new OutputStreamWriter(work.getOutputStream(), StandardCharsets.UTF_8);
            JSONObject wa = new JSONObject();
            wa.put("type", "work_auth");
            wa.put("token", token);
            wa.put("conn_id", connID);
            writeMsg(ww, wa);

            final Socket l = local, w2 = work;
            pool.execute(() -> pump(w2, l, true));   // 服务端 -> 本地
            pump(l, w2, false);                       // 本地 -> 服务端（当前线程）
        } catch (Exception e) {
            listener.onLog("隧道 " + t.name + " 连接失败：" + e.getMessage());
            closeQuietly(local);
            closeQuietly(work);
        }
    }

    /** 双向搬运字节，并统计流量。 */
    private void pump(Socket from, Socket to, boolean isDown) {
        byte[] buf = new byte[32 * 1024];
        try (InputStream in = from.getInputStream(); OutputStream out = to.getOutputStream()) {
            int n;
            while (running.get() && (n = in.read(buf)) > 0) {
                out.write(buf, 0, n);
                out.flush();
                if (isDown) downBytes += n; else upBytes += n;
            }
        } catch (Exception ignored) {
        } finally {
            closeQuietly(from);
            closeQuietly(to);
        }
    }

    // ---------------- UDP 中继隧道 ----------------

    private void openUdpRelay(Tunnel t) {
        pool.execute(() -> {
            while (running.get() && tunnelSnapshot().contains(t)) {
                Socket s = null;
                try {
                    final Socket sock = new Socket();
                    s = sock;
                    sock.connect(new InetSocketAddress(serverHost, Integer.parseInt(workPort)), 10000);
                    Writer ww = new OutputStreamWriter(sock.getOutputStream(), StandardCharsets.UTF_8);
                    JSONObject reg = new JSONObject();
                    reg.put("type", "udp_register");
                    reg.put("token", token);
                    reg.put("tunnel_id", t.id);
                    writeMsg(ww, reg);
                    synchronized (udpRelays) {
                        udpRelays.put(t.id, sock);
                    }
                    listener.onLog("UDP 中继已建立：" + t.name);

                    BufferedReader in = new BufferedReader(
                            new InputStreamReader(sock.getInputStream(), StandardCharsets.UTF_8));
                    String line;
                    while (running.get() && (line = in.readLine()) != null) {
                        if (line.trim().isEmpty()) continue;
                        JSONObject m = new JSONObject(line);
                        if ("udp_data".equals(m.optString("type"))) {
                            String from = m.optString("from");
                            byte[] data = Base64.decode(m.optString("data", ""), Base64.DEFAULT);
                            pool.execute(() -> forwardUdp(t, from, data, sock));
                        }
                    }
                } catch (Exception e) {
                    if (running.get()) listener.onLog("UDP 中继异常：" + e.getMessage());
                } finally {
                    synchronized (udpRelays) {
                        udpRelays.remove(t.id);
                    }
                    closeQuietly(s);
                }
                if (running.get()) sleep(3000);
            }
        });
    }

    private void forwardUdp(Tunnel t, String from, byte[] data, Socket relay) {
        final Socket relayFinal = relay;
        try (DatagramSocket local = new DatagramSocket()) {
            local.connect(new InetSocketAddress(t.localHost, t.localPort));
            local.setSoTimeout(2000);
            local.send(new DatagramPacket(data, data.length));
            byte[] rb = new byte[65535];
            DatagramPacket rp = new DatagramPacket(rb, rb.length);
            local.receive(rp);
            upBytes += rp.getLength();
            JSONObject m = new JSONObject();
            m.put("type", "udp_data");
            m.put("tunnel_id", t.id);
            m.put("from", from);
            m.put("data", Base64.encodeToString(rp.getData(), 0, rp.getLength(), Base64.NO_WRAP));
            Writer ww = new OutputStreamWriter(relayFinal.getOutputStream(), StandardCharsets.UTF_8);
            synchronized (udpRelays) {
                writeMsg(ww, m);
            }
        } catch (Exception ignored) {
        }
    }

    // ---------------- 工具 ----------------

    private static synchronized void writeMsg(Writer w, JSONObject o) throws Exception {
        w.write(o.toString());
        w.write('\n');
        w.flush();
    }

    private static JSONObject readMsg(BufferedReader in) throws Exception {
        String line = in.readLine();
        if (line == null) throw new Exception("服务端关闭连接");
        return new JSONObject(line);
    }

    private static void closeQuietly(Socket s) {
        if (s != null) try { s.close(); } catch (Exception ignored) { }
    }

    private static void sleep(long ms) {
        try { Thread.sleep(ms); } catch (InterruptedException ignored) { Thread.currentThread().interrupt(); }
    }

    /** 解析服务端地址字符串 "host:port"。 */
    public static String[] parseServer(String input) {
        String s = input.trim();
        if (s.startsWith("http://")) s = s.substring(7);
        if (s.startsWith("https://")) s = s.substring(8);
        int slash = s.indexOf('/');
        if (slash > 0) s = s.substring(0, slash);
        int colon = s.lastIndexOf(':');
        if (colon < 0) return new String[]{s, "7000"};
        return new String[]{s.substring(0, colon), s.substring(colon + 1)};
    }
}