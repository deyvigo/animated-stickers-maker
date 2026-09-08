package expo.modules.whatsappstickers

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

/**
 * Reads `packs.json`, written by the JS side (see
 * app/src/features/packs/storage.ts — keep the two schemas in sync).
 *
 * Location: `${context.filesDir}/stickers/packs.json`, with each pack's
 * images under `${context.filesDir}/stickers/<identifier>/<file>`. This is
 * exactly what Expo's new FileSystem `Paths.document` resolves to for a
 * dev-client / standalone Android build (not Expo Go, which sandboxes
 * documentDirectory differently) — see the JS storage module's comment.
 */
data class StickerFile(val imageFile: String, val emojis: List<String>, val accessibilityText: String)

data class StickerPackData(
  val identifier: String,
  val name: String,
  val publisher: String,
  val trayImageFile: String,
  val animatedStickerPack: Boolean,
  val publisherEmail: String,
  val publisherWebsite: String,
  val privacyPolicyWebsite: String,
  val licenseAgreementWebsite: String,
  val imageDataVersion: String,
  val avoidCache: Boolean,
  val stickers: List<StickerFile>,
)

class StickerPackRepository(private val context: Context) {

  private fun stickersRoot(): File = File(context.filesDir, "stickers")

  private fun packsJsonFile(): File = File(stickersRoot(), "packs.json")

  fun packDirectory(identifier: String): File = File(stickersRoot(), identifier)

  fun assetFile(identifier: String, fileName: String): File = File(packDirectory(identifier), fileName)

  fun readAllPacks(): List<StickerPackData> {
    val file = packsJsonFile()
    if (!file.exists()) return emptyList()
    return try {
      val root = JSONObject(file.readText())
      val packsArray = root.optJSONArray("packs") ?: JSONArray()
      (0 until packsArray.length()).mapNotNull { i -> parsePack(packsArray.optJSONObject(i)) }
    } catch (_: Exception) {
      // A malformed packs.json must not crash WhatsApp's query — just look
      // like an app with no packs yet.
      emptyList()
    }
  }

  fun readPack(identifier: String): StickerPackData? = readAllPacks().find { it.identifier == identifier }

  private fun parsePack(obj: JSONObject?): StickerPackData? {
    if (obj == null) return null
    val identifier = obj.optString("identifier")
    if (identifier.isEmpty()) return null
    val stickersArray = obj.optJSONArray("stickers") ?: JSONArray()
    val stickers = (0 until stickersArray.length()).mapNotNull { i ->
      val s = stickersArray.optJSONObject(i) ?: return@mapNotNull null
      val imageFile = s.optString("imageFile")
      if (imageFile.isEmpty()) return@mapNotNull null
      val emojisArray = s.optJSONArray("emojis") ?: JSONArray()
      val emojis = (0 until emojisArray.length()).map { emojisArray.optString(it) }
      StickerFile(imageFile, emojis, s.optString("accessibilityText", ""))
    }
    return StickerPackData(
      identifier = identifier,
      name = obj.optString("name", "Stickers"),
      publisher = obj.optString("publisher", ""),
      trayImageFile = obj.optString("trayImageFile", ""),
      animatedStickerPack = obj.optBoolean("animatedStickerPack", false),
      publisherEmail = obj.optString("publisherEmail", ""),
      publisherWebsite = obj.optString("publisherWebsite", ""),
      privacyPolicyWebsite = obj.optString("privacyPolicyWebsite", ""),
      licenseAgreementWebsite = obj.optString("licenseAgreementWebsite", ""),
      imageDataVersion = obj.optString("imageDataVersion", "1"),
      avoidCache = obj.optBoolean("avoidCache", false),
      stickers = stickers,
    )
  }
}
