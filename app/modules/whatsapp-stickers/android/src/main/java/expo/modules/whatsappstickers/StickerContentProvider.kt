package expo.modules.whatsappstickers

import android.content.ContentProvider
import android.content.ContentValues
import android.content.UriMatcher
import android.content.res.AssetFileDescriptor
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.os.ParcelFileDescriptor
import java.io.FileNotFoundException

/**
 * Serves this app's locally-generated sticker packs to WhatsApp, per
 * WhatsApp's third-party sticker contract
 * (https://github.com/WhatsApp/stickers — the column names and URI shapes
 * below come straight from that spec and haven't changed in years).
 *
 * Unlike the official sample (which bundles fixed packs under `assets/`),
 * this provider reads packs generated at runtime from
 * `${filesDir}/stickers/packs.json` via [StickerPackRepository], because
 * packs here come from the user cropping TikTok videos, not from anything
 * shipped at build time.
 *
 * Registered in AndroidManifest.xml (this module's own library manifest)
 * with authority `${applicationId}.stickercontentprovider` — that must
 * match STICKER_PACK_AUTHORITY used in WhatsappStickersModule.kt.
 */
class StickerContentProvider : ContentProvider() {

  companion object {
    private const val METADATA = "metadata"
    private const val METADATA_CODE = 1
    private const val METADATA_CODE_FOR_SINGLE_PACK = 2
    private const val STICKERS = "stickers"
    private const val STICKERS_CODE = 3
    private const val STICKERS_ASSET = "stickers_asset"
    private const val STICKERS_ASSET_CODE = 4

    // --- metadata cursor columns (exact names WhatsApp expects) ---
    private const val STICKER_PACK_IDENTIFIER = "sticker_pack_identifier"
    private const val STICKER_PACK_NAME = "sticker_pack_name"
    private const val STICKER_PACK_PUBLISHER = "sticker_pack_publisher"
    private const val STICKER_PACK_ICON = "sticker_pack_icon"
    private const val ANDROID_APP_DOWNLOAD_LINK = "android_play_store_link"
    private const val IOS_APP_DOWNLOAD_LINK = "ios_app_download_link"
    private const val PUBLISHER_EMAIL = "sticker_pack_publisher_email"
    private const val PUBLISHER_WEBSITE = "sticker_pack_publisher_website"
    private const val PRIVACY_POLICY_WEBSITE = "sticker_pack_privacy_policy_website"
    private const val LICENSE_AGREEMENT_WEBSITE = "sticker_pack_license_agreement_website"
    private const val IMAGE_DATA_VERSION = "image_data_version"
    private const val AVOID_CACHE = "whatsapp_will_not_cache_stickers"
    private const val ANIMATED_STICKER_PACK = "animated_sticker_pack"

    // --- stickers cursor columns ---
    private const val STICKER_FILE_NAME = "sticker_file_name"
    private const val STICKER_FILE_EMOJI = "sticker_emoji"
    private const val STICKER_FILE_ACCESSIBILITY_TEXT = "sticker_accessibility_text"

    private val METADATA_COLUMNS = arrayOf(
      STICKER_PACK_IDENTIFIER, STICKER_PACK_NAME, STICKER_PACK_PUBLISHER, STICKER_PACK_ICON,
      ANDROID_APP_DOWNLOAD_LINK, IOS_APP_DOWNLOAD_LINK, PUBLISHER_EMAIL, PUBLISHER_WEBSITE,
      PRIVACY_POLICY_WEBSITE, LICENSE_AGREEMENT_WEBSITE, IMAGE_DATA_VERSION, AVOID_CACHE,
      ANIMATED_STICKER_PACK,
    )
    private val STICKERS_COLUMNS = arrayOf(STICKER_FILE_NAME, STICKER_FILE_EMOJI, STICKER_FILE_ACCESSIBILITY_TEXT)
  }

  private val matcher: UriMatcher by lazy {
    val authority = "${context!!.packageName}.stickercontentprovider"
    UriMatcher(UriMatcher.NO_MATCH).apply {
      addURI(authority, METADATA, METADATA_CODE)
      addURI(authority, "$METADATA/*", METADATA_CODE_FOR_SINGLE_PACK)
      addURI(authority, "$STICKERS/*", STICKERS_CODE)
      addURI(authority, "$STICKERS_ASSET/*/*", STICKERS_ASSET_CODE)
    }
  }

  private val repository: StickerPackRepository by lazy { StickerPackRepository(context!!) }

  override fun onCreate(): Boolean = true

  override fun getType(uri: Uri): String {
    val authority = "${context!!.packageName}.stickercontentprovider"
    return when (matcher.match(uri)) {
      METADATA_CODE -> "vnd.android.cursor.dir/vnd.$authority.$METADATA"
      METADATA_CODE_FOR_SINGLE_PACK -> "vnd.android.cursor.item/vnd.$authority.$METADATA"
      STICKERS_CODE -> "vnd.android.cursor.dir/vnd.$authority.$STICKERS"
      STICKERS_ASSET_CODE -> {
        val fileName = uri.lastPathSegment ?: ""
        if (fileName.endsWith(".webp")) "image/webp" else "image/png"
      }
      else -> throw IllegalArgumentException("Unsupported URI: $uri")
    }
  }

  override fun query(
    uri: Uri,
    projection: Array<out String>?,
    selection: String?,
    selectionArgs: Array<out String>?,
    sortOrder: String?,
  ): Cursor {
    val cursor = when (matcher.match(uri)) {
      METADATA_CODE -> metadataCursor(null)
      METADATA_CODE_FOR_SINGLE_PACK -> metadataCursor(uri.lastPathSegment)
      STICKERS_CODE -> stickersCursor(uri.lastPathSegment)
      else -> throw IllegalArgumentException("Unsupported URI for query: $uri")
    }
    // Lets WhatsApp's ContentObserver notice when packs.json changes (e.g. a
    // new sticker added after the pack was already installed), matching the
    // official sample's behavior. Harmless if nothing ever observes it.
    cursor.setNotificationUri(context!!.contentResolver, uri)
    return cursor
  }

  private fun metadataCursor(identifier: String?): Cursor {
    val cursor = MatrixCursor(METADATA_COLUMNS)
    val packs = if (identifier == null) repository.readAllPacks() else listOfNotNull(repository.readPack(identifier))
    for (pack in packs) {
      cursor.addRow(
        arrayOf(
          pack.identifier,
          pack.name,
          pack.publisher,
          pack.trayImageFile,
          "", // no Play Store listing for this personal-use pack
          "", // no iOS app
          pack.publisherEmail,
          pack.publisherWebsite,
          pack.privacyPolicyWebsite,
          pack.licenseAgreementWebsite,
          pack.imageDataVersion,
          if (pack.avoidCache) 1 else 0,
          if (pack.animatedStickerPack) 1 else 0,
        ),
      )
    }
    return cursor
  }

  private fun stickersCursor(identifier: String?): Cursor {
    val cursor = MatrixCursor(STICKERS_COLUMNS)
    val pack = identifier?.let { repository.readPack(it) }
    pack?.stickers?.forEach { sticker ->
      cursor.addRow(arrayOf(sticker.imageFile, sticker.emojis.joinToString(","), sticker.accessibilityText))
    }
    return cursor
  }

  override fun openAssetFile(uri: Uri, mode: String): AssetFileDescriptor {
    if (matcher.match(uri) != STICKERS_ASSET_CODE) {
      throw IllegalArgumentException("Unsupported URI for asset: $uri")
    }
    val segments = uri.pathSegments // [stickers_asset, <identifier>, <fileName>]
    if (segments.size != 3) {
      throw IllegalArgumentException("Malformed asset URI: $uri")
    }
    val identifier = segments[1]
    val fileName = segments[2]
    val file = repository.assetFile(identifier, fileName)
    if (!file.exists()) {
      throw FileNotFoundException("Asset not found: $identifier/$fileName")
    }
    val pfd = ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
    return AssetFileDescriptor(pfd, 0, AssetFileDescriptor.UNKNOWN_LENGTH)
  }

  // Read-only content provider: WhatsApp only ever queries and reads
  // assets from this authority, it never writes back.
  override fun insert(uri: Uri, values: ContentValues?): Uri? =
    throw UnsupportedOperationException("This content provider is read-only")

  override fun update(uri: Uri, values: ContentValues?, selection: String?, selectionArgs: Array<out String>?): Int =
    throw UnsupportedOperationException("This content provider is read-only")

  override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?): Int =
    throw UnsupportedOperationException("This content provider is read-only")
}
