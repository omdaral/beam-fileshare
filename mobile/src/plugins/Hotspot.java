package app.beam.share;

import android.content.pm.PackageManager;
import android.net.wifi.WifiConfiguration;
import android.net.wifi.WifiManager;
import android.net.wifi.SoftApConfiguration;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;

import androidx.core.app.ActivityCompat;
import androidx.core.content.ContextCompat;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

/**
 * Phone hotspot via Android Local-Only Hotspot (no internet needed —
 * perfect for Beam). Returns the OS-chosen SSID/password so the Beam
 * page can QR-share them. Falls back gracefully on old/odd devices.
 */
@CapacitorPlugin(name = "Hotspot")
public class Hotspot extends Plugin {
    private WifiManager.LocalOnlyHotspotReservation reservation;
    private PluginCall pendingCall;

    private boolean hasPerms() {
        if (Build.VERSION.SDK_INT >= 33) {
            return ContextCompat.checkSelfPermission(getContext(),
                    android.Manifest.permission.NEARBY_WIFI_DEVICES)
                    == PackageManager.PERMISSION_GRANTED;
        }
        return ContextCompat.checkSelfPermission(getContext(),
                android.Manifest.permission.ACCESS_FINE_LOCATION)
                == PackageManager.PERMISSION_GRANTED;
    }

    private void requestPerms() {
        if (Build.VERSION.SDK_INT >= 33) {
            ActivityCompat.requestPermissions(getActivity(),
                    new String[]{android.Manifest.permission.NEARBY_WIFI_DEVICES}, 4781);
        } else {
            ActivityCompat.requestPermissions(getActivity(),
                    new String[]{android.Manifest.permission.ACCESS_FINE_LOCATION}, 4781);
        }
    }

    @PluginMethod
    public void startLocalOnly(PluginCall call) {
        if (Build.VERSION.SDK_INT < 26) {
            call.reject("needs_android_8");
            return;
        }
        if (!hasPerms()) {
            pendingCall = call;
            requestPerms();
            return; // JS retries after the permission dialog
        }
        startReservation(call);
    }

    private void startReservation(final PluginCall call) {
        WifiManager wm = (WifiManager) getContext().getApplicationContext()
                .getSystemService(android.content.Context.WIFI_SERVICE);
        if (wm == null) {
            call.reject("no_wifi");
            return;
        }
        try {
            stopReservation();
            wm.startLocalOnlyHotspot(new WifiManager.LocalOnlyHotspotCallback() {
                @Override
                public void onStarted(WifiManager.LocalOnlyHotspotReservation res) {
                    reservation = res;
                    JSObject ret = new JSObject();
                    String ssid = "";
                    String pass = "";
                    String security = "wpa";
                    try {
                        if (Build.VERSION.SDK_INT >= 30) {
                            SoftApConfiguration cfg = res.getSoftApConfiguration();
                            if (cfg != null) {
                                if (cfg.getSsid() != null) ssid = cfg.getSsid();
                                if (cfg.getPassphrase() != null) pass = cfg.getPassphrase();
                                if (cfg.getSecurityType() == SoftApConfiguration.SECURITY_TYPE_OPEN)
                                    security = "open";
                            }
                        } else {
                            @SuppressWarnings("deprecation")
                            WifiConfiguration cfg = res.getWifiConfiguration();
                            if (cfg != null) {
                                if (cfg.SSID != null) ssid = cfg.SSID.replace("\"", "");
                                if (cfg.preSharedKey != null) pass = cfg.preSharedKey.replace("\"", "");
                            }
                        }
                    } catch (Exception e) {
                        call.reject("creds_unreadable");
                        return;
                    }
                    ret.put("ssid", ssid);
                    ret.put("pass", pass);
                    ret.put("security", security);
                    call.resolve(ret);
                }

                @Override
                public void onStopped() {
                    reservation = null;
                }

                @Override
                public void onFailed(int reason) {
                    call.reject("hotspot_failed:" + reason);
                }
            }, new Handler(Looper.getMainLooper()));
        } catch (SecurityException se) {
            pendingCall = call;
            requestPerms();
        } catch (Exception e) {
            call.reject("hotspot_error");
        }
    }

    @PluginMethod
    public void stop(PluginCall call) {
        stopReservation();
        call.resolve();
    }

    private void stopReservation() {
        if (reservation != null) {
            try {
                reservation.close();
            } catch (Exception ignored) {
            }
            reservation = null;
        }
    }

    @Override
    protected void handleOnDestroy() {
        stopReservation();
    }
}
