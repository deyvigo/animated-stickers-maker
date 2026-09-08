package expo.modules.whatsappstickers

import android.content.ActivityNotFoundException
import android.content.ContentResolver
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.net.Uri
import expo.modules.kotlin.Promise
import expo.modules.kotlin.exception.CodedException
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

/**
 * Bridges to WhatsApp's third-party sticker API
 * (https://github.com/WhatsApp/stickers/tree/main/Android). The constants
 * and query shapes below were verified against that repo's
 * WhitelistCheck.java / AddStickerPackActivity.java / StickerPackDetailsActivity.java
 * on 2026-09-07 — WhatsApp doesn't publish this as a formal API, so if a
 * future WhatsApp update changes it, re-check against that source.
 */
private const val CONSUMER_PACKAGE = "com.whatsapp"
private const val BUSINESS_PACKAGE = "com.whatsapp.w4b"
private const val ENABLE_STICKER_PACK_ACTION = "com.whatsapp.intent.action.ENABLE_STICKER_PACK"
private const val EXTRA_STICKER_PACK_ID = "sticker_pack_id"
private const val EXTRA_STICKER_PACK_AUTHORITY = "sticker_pack_authority"
private const val EXTRA_STICKER_PACK_NAME = "sticker_pack_name"
private const val ADD_PACK_REQUEST_CODE = 2846 // arbitrary, just needs to be stable for this module

class WhatsappStickersModule : Module() {
  private var pendingAddPackPromise: Promise? = null

  private val authority: String
    get() = "${appContext.reactContext!!.packageName}.stickercontentprovider"

  override fun definition() = ModuleDefinition {
    Name("WhatsappStickers")

    AsyncFunction("isWhatsAppInstalled") {
      val pm = appContext.reactContext!!.packageManager
      mapOf(
        "consumer" to isPackageInstalled(pm, CONSUMER_PACKAGE),
        "business" to isPackageInstalled(pm, BUSINESS_PACKAGE),
      )
    }

    AsyncFunction("isPackAdded") { packId: String ->
      val context = appContext.reactContext!!
      val pm = context.packageManager
      val consumerInstalled = isPackageInstalled(pm, CONSUMER_PACKAGE)
      val businessInstalled = isPackageInstalled(pm, BUSINESS_PACKAGE)
      if (!consumerInstalled && !businessInstalled) return@AsyncFunction false
      try {
        val consumerOk = !consumerInstalled || isWhitelistedIn(context, packId, CONSUMER_PACKAGE)
        val businessOk = !businessInstalled || isWhitelistedIn(context, packId, BUSINESS_PACKAGE)
        consumerOk && businessOk
      } catch (_: Exception) {
        // Best-effort: WhatsApp doesn't document this query, so any failure
        // (e.g. a WhatsApp version that changed it) just falls back to "not
        // known to be added" rather than crashing the app.
        false
      }
    }

    AsyncFunction("addPackToWhatsApp") { packId: String, promise: Promise ->
      val context = appContext.reactContext!!
      val activity = appContext.currentActivity
      if (activity == null) {
        promise.reject(CodedException("No hay una Activity disponible para abrir WhatsApp."))
        return@AsyncFunction
      }
      val pm = context.packageManager
      val consumerInstalled = isPackageInstalled(pm, CONSUMER_PACKAGE)
      val businessInstalled = isPackageInstalled(pm, BUSINESS_PACKAGE)
      if (!consumerInstalled && !businessInstalled) {
        promise.reject(CodedException("WhatsApp no está instalado en este dispositivo."))
        return@AsyncFunction
      }

      val pack = StickerPackRepository(context).readPack(packId)
      if (pack == null) {
        promise.reject(CodedException("No existe un pack local con id $packId."))
        return@AsyncFunction
      }

      val intent = Intent().apply {
        action = ENABLE_STICKER_PACK_ACTION
        putExtra(EXTRA_STICKER_PACK_ID, packId)
        putExtra(EXTRA_STICKER_PACK_AUTHORITY, authority)
        putExtra(EXTRA_STICKER_PACK_NAME, pack.name)
      }

      // If only one of the two WhatsApp variants is installed, target it
      // directly; otherwise let the user pick, same as the official sample.
      if (consumerInstalled && !businessInstalled) {
        intent.setPackage(CONSUMER_PACKAGE)
      } else if (businessInstalled && !consumerInstalled) {
        intent.setPackage(BUSINESS_PACKAGE)
      }

      pendingAddPackPromise = promise
      try {
        if (consumerInstalled && businessInstalled) {
          activity.startActivityForResult(Intent.createChooser(intent, "Agregar a WhatsApp"), ADD_PACK_REQUEST_CODE)
        } else {
          activity.startActivityForResult(intent, ADD_PACK_REQUEST_CODE)
        }
      } catch (e: ActivityNotFoundException) {
        pendingAddPackPromise = null
        promise.reject(CodedException("WhatsApp no pudo abrir el diálogo para agregar el pack.", e))
      }
    }

    OnActivityResult { _, payload ->
      if (payload.requestCode != ADD_PACK_REQUEST_CODE) return@OnActivityResult
      val promise = pendingAddPackPromise ?: return@OnActivityResult
      pendingAddPackPromise = null

      if (payload.resultCode == android.app.Activity.RESULT_OK) {
        promise.resolve("added")
      } else {
        val validationError = payload.data?.getStringExtra("validation_error")
        if (validationError != null) {
          promise.reject(CodedException("WhatsApp rechazó el pack: $validationError"))
        } else {
          promise.resolve("cancelled")
        }
      }
    }
  }

  private fun isPackageInstalled(pm: PackageManager, packageName: String): Boolean {
    return try {
      val info: ApplicationInfo = pm.getApplicationInfo(packageName, 0)
      info.enabled
    } catch (_: PackageManager.NameNotFoundException) {
      false
    }
  }

  /** See WhatsApp/stickers' WhitelistCheck.java `isWhitelistedFromProvider` —
   * queries `content://<whatsappPackage>.provider.sticker_whitelist_check/is_whitelisted`
   * with `authority`/`identifier` query params, single-column "result" (0/1). */
  private fun isWhitelistedIn(context: android.content.Context, packId: String, whatsappPackage: String): Boolean {
    val providerAuthority = "$whatsappPackage.provider.sticker_whitelist_check"
    val pm = context.packageManager
    if (pm.resolveContentProvider(providerAuthority, 0) == null) {
      // Provider missing means an old WhatsApp version that predates this
      // check; treat as "not confirmed" rather than throwing.
      return false
    }
    val uri = Uri.Builder()
      .scheme(ContentResolver.SCHEME_CONTENT)
      .authority(providerAuthority)
      .appendPath("is_whitelisted")
      .appendQueryParameter("authority", authority)
      .appendQueryParameter("identifier", packId)
      .build()
    context.contentResolver.query(uri, null, null, null, null)?.use { cursor ->
      if (cursor.moveToFirst()) {
        val columnIndex = cursor.getColumnIndex("result")
        if (columnIndex >= 0) {
          return cursor.getInt(columnIndex) == 1
        }
      }
    }
    return false
  }
}
