<script>
import Banner from "@components/Banner/Banner";
import { LabeledInput } from "@components/Form/LabeledInput";
import { Checkbox } from "@components/Form/Checkbox";
import { SECRET } from "@shell/config/types";
import LabeledSelect from "@shell/components/form/LabeledSelect";

import {
  decodeCredential,
  errorText,
  exchangeToken,
  itemsOf,
  proxyGet,
  resourceOption,
  signServiceAccountJWT,
  nameOf,
} from "../utils/nebius";

const DISK_TYPES = [
  "NETWORK_SSD",
  "NETWORK_SSD_NON_REPLICATED",
  "NETWORK_SSD_IO_M3",
  "NETWORK_HDD",
];

export default {
  components: {
    Banner,
    Checkbox,
    LabeledInput,
    LabeledSelect,
  },

  props: {
    uuid: {
      type:     String,
      required: true,
    },
    mode: {
      type:     String,
      required: true,
    },
    value: {
      type:     Object,
      required: true,
    },
    credentialId: {
      type:    String,
      default: "",
    },
  },

  async fetch() {
    await this.fetchCredential();
    await this.loadLists();
  },

  data() {
    return {
      credential: null,
      token:      "",

      subnets:         null,
      securityGroups:  null,
      images:          null,
      platforms:       null,
      presets:         null,

      listError: "",
    };
  },

  created() {
    this.value.subnetId = this.value.subnetId ?? "";
    this.value.imageFamily = this.value.imageFamily ?? "";
    this.value.imageId = this.value.imageId ?? "";
    this.value.platform = this.value.platform ?? "";
    this.value.preset = this.value.preset ?? "";

    this.value.diskSizeGib = (this.value.diskSizeGib ?? "").toString() || "50";
    this.value.diskType = this.value.diskType || "NETWORK_SSD";
    this.value.publicIp = this.value.publicIp || "static";

    const groups = this.value.securityGroups;

    if (Array.isArray(groups)) {
      this.value.securityGroups = groups;
    } else if (typeof groups === "string" && groups.trim() !== "") {
      this.value.securityGroups = this.splitList(groups);
    } else {
      this.value.securityGroups = [];
    }

    this.value.preemptible = this.value.preemptible ?? false;
    this.value.sshUser = this.value.sshUser || "ubuntu";
    this.value.sshPort = (this.value.sshPort ?? "").toString() || "22";
    this.value.createTimeout = this.value.createTimeout || "20m";
    this.value.deleteTimeout = this.value.deleteTimeout || "5m";

    this.validate();
  },

  watch: {
    async credentialId() {
      await this.$fetch();
      this.validate();
    },
    "value.platform"(neu) {
      if (this.token && neu) {
        this.fetchPresets();
      } else if (!neu) {
        this.presets = null;
      }
    },
    value: {
      deep:     true,
      handler() {
        this.validate();
      },
    },
  },

  computed: {
    disabled() {
      return this.mode === "view";
    },

    diskTypeOptions() {
      return DISK_TYPES.map((t) => ({ label: t, value: t }));
    },

    imageConflict() {
      return !!this.value.imageFamily && !!this.value.imageId;
    },

    publicIpOptions() {
      return ["static", "dynamic", "none"].map((v) => ({
        label: this.t(`cluster.machineConfig.nebius.publicIp.options.${ v }`),
        value: v,
      }));
    },

    subnetOptions() {
      return this.optionsFrom(this.subnets);
    },

    securityGroupOptions() {
      return this.optionsFrom(this.securityGroups);
    },

    imageFamilyOptions() {
      if (!this.images) {
        return null;
      }

      const families = Array.from(
        new Set(
          this.images
            .map((i) => i.family || i.spec?.family || "")
            .filter((f) => typeof f === "string" && f.trim() !== "")
        )
      ).sort();

      if (families.length === 0) {
        return null;
      }

      return families.map((f) => ({ label: f, value: f }));
    },

    imageOptions() {
      return this.optionsFrom(this.images);
    },

    platformOptions() {
      return this.optionsFrom(this.platforms);
    },

    presetOptions() {
      if (!this.presets) {
        return null;
      }

      const platform = this.value.platform;
      const filtered = this.presets.filter((p) => {
        const pPlatform = p.platform || p.spec?.platform || "";

        return !pPlatform || !platform || pPlatform === platform;
      });

      return filtered
        .map((p) => ({ label: nameOf(p), value: nameOf(p) }))
        .filter((o) => o.value)
        .sort((a, b) => a.label.localeCompare(b.label));
    },
  },

  methods: {
    splitList(value) {
      return (value || "")
        .split(/[\n,]+/)
        .map((s) => s.trim())
        .filter(Boolean);
    },

    optionsFrom(list) {
      if (!list || list.length === 0) {
        return null;
      }

      return list
        .map((x) => resourceOption(x))
        .filter((o) => o.value)
        .sort((a, b) => a.label.localeCompare(b.label));
    },

    mapKeyError(e) {
      if (e?.message === "invalid-key") {
        return this.t("cluster.nebius.errors.invalidPrivateKey");
      }
      if (e?.message === "pkcs1-key") {
        return this.t("cluster.nebius.errors.pkcs1PrivateKey");
      }
      if (e?.message === "no-token") {
        return this.t("cluster.nebius.errors.tokenResponse");
      }

      return "";
    },

    async fetchCredential() {
      if (!this.credentialId) {
        this.credential = null;
        return;
      }

      try {
        this.listError = "";

        const secret = await this.$store.dispatch("management/find", {
          type: SECRET,
          id:   this.credentialId.replace(":", "/"),
        });

        this.credential = decodeCredential(secret);
      } catch (e) {
        // Never log the secret object: only its mapped message.
        this.credential = null;
        this.listError = errorText(
          this.t,
          e,
          "cluster.nebius.errors.listsUnavailable"
        );
      }
    },

    async loadLists() {
      if (!this.credential || !this.credential.privateKey || !this.credential.projectId) {
        return;
      }

      try {
        const assertion = await signServiceAccountJWT({
          privateKey:  this.credential.privateKey,
          saId:        this.credential.saId,
          publicKeyId: this.credential.publicKeyId,
        });

        this.token = await exchangeToken(this.$store, { assertion });
      } catch (e) {
        this.token = "";
        this.listError =
          this.mapKeyError(e) ||
          errorText(this.t, e, "cluster.nebius.errors.listsUnavailable");
        return;
      }

      await Promise.all([
        this.fetchList("subnets", "/vpc/v1/subnets", { parent_id: this.credential.projectId }),
        this.fetchList("securityGroups", "/vpc/v1/security-groups", { parent_id: this.credential.projectId }),
        this.fetchList("images", "/compute/v1/images", { parent_id: this.credential.projectId }),
        this.fetchList("platforms", "/compute/v1/platforms", {}),
      ]);

      if (this.value.platform) {
        await this.fetchPresets();
      }
    },

    async fetchList(field, path, query) {
      try {
        const res = await proxyGet(this.$store, {
          apiEndpoint: this.credential.apiEndpoint,
          token:       this.token,
          path,
          query,
        });

        this[field] = itemsOf(res);
      } catch (e) {
        this[field] = null;
        this.listError =
          this.listError ||
          errorText(this.t, e, "cluster.nebius.errors.listsUnavailable");
      }
    },

    async fetchPresets() {
      if (!this.token || !this.credential || !this.value.platform) {
        return;
      }

      try {
        const res = await proxyGet(this.$store, {
          apiEndpoint: this.credential.apiEndpoint,
          token:       this.token,
          path:        "/compute/v1/presets",
          query:       { parent_id: this.credential.projectId },
        });

        this.presets = itemsOf(res);
      } catch (e) {
        this.presets = null;
        this.listError =
          this.listError ||
          errorText(this.t, e, "cluster.nebius.errors.listsUnavailable");
      }
    },

    durationMs(value) {
      const m = /^(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)$/.exec((value || "").trim());

      if (!m) {
        return -1;
      }

      const mult = {
        ns: 1e-6, us: 1e-3, "µs": 1e-3, ms: 1, s: 1000, m: 60000, h: 3600000,
      }[m[2]];

      return parseFloat(m[1]) * mult;
    },

    validate() {
      const hasImage =
        (!!this.value.imageFamily && !!this.value.imageId) ||
        (!this.value.imageFamily && !this.value.imageId);

      if (
        !this.value.subnetId ||
        hasImage ||
        !this.value.platform ||
        !this.value.preset ||
        !this.value.diskSizeGib ||
        parseInt(this.value.diskSizeGib, 10) < 1 ||
        !this.value.sshUser ||
        !this.value.sshPort ||
        parseInt(this.value.sshPort, 10) < 1 ||
        parseInt(this.value.sshPort, 10) > 65535 ||
        this.durationMs(this.value.createTimeout) <= 0 ||
        this.durationMs(this.value.deleteTimeout) <= 0
      ) {
        this.$emit("validationChanged", false);
        return;
      }

      this.$emit("validationChanged", true);
    },
  },
};
</script>

<template>
  <div>
    <Banner v-if="listError" class="mb-20" color="warning">
      <div>{{ listError }}</div>
      <div>
        {{ t("cluster.machineConfig.nebius.errors.listsFallback") }}
      </div>
    </Banner>

    <Banner v-if="imageConflict" class="mb-20" color="warning">
      {{ t("cluster.machineConfig.nebius.image.conflict") }}
    </Banner>

    <h3>
      <t k="cluster.machineConfig.nebius.instance.header" />
    </h3>

    <div class="row mb-20">
      <div class="col span-6">
        <LabeledSelect
          v-if="subnetOptions"
          v-model:value="value.subnetId"
          :mode="mode"
          :disabled="disabled"
          :options="subnetOptions"
          :searchable="true"
          label-key="cluster.machineConfig.nebius.subnetId.label"
          required
        />
        <LabeledInput
          v-else
          :mode="mode"
          :disabled="disabled"
          :value="value.subnetId"
          label-key="cluster.machineConfig.nebius.subnetId.label"
          placeholder-key="cluster.machineConfig.nebius.subnetId.placeholder"
          required
          @change="
            (e) => {
              value.subnetId = e.target.value;
            }
          "
        />
      </div>
      <div class="col span-6">
        <LabeledSelect
          v-if="platformOptions"
          v-model:value="value.platform"
          :mode="mode"
          :disabled="disabled"
          :options="platformOptions"
          :searchable="true"
          label-key="cluster.machineConfig.nebius.platform.label"
          required
        />
        <LabeledInput
          v-else
          :mode="mode"
          :disabled="disabled"
          :value="value.platform"
          label-key="cluster.machineConfig.nebius.platform.label"
          placeholder-key="cluster.machineConfig.nebius.platform.placeholder"
          required
          @change="
            (e) => {
              value.platform = e.target.value;
            }
          "
        />
      </div>
    </div>

    <div class="row mb-20">
      <div class="col span-6">
        <LabeledSelect
          v-if="presetOptions"
          v-model:value="value.preset"
          :mode="mode"
          :disabled="disabled || !value.platform"
          :options="presetOptions"
          :searchable="true"
          label-key="cluster.machineConfig.nebius.preset.label"
          required
        />
        <LabeledInput
          v-else
          :mode="mode"
          :disabled="disabled"
          :value="value.preset"
          label-key="cluster.machineConfig.nebius.preset.label"
          placeholder-key="cluster.machineConfig.nebius.preset.placeholder"
          required
          @change="
            (e) => {
              value.preset = e.target.value;
            }
          "
        />
      </div>
      <div class="col span-6">
        <LabeledSelect
          v-if="imageFamilyOptions"
          v-model:value="value.imageFamily"
          :mode="mode"
          :disabled="disabled"
          :options="imageFamilyOptions"
          :searchable="true"
          label-key="cluster.machineConfig.nebius.imageFamily.label"
          tooltip-key="cluster.machineConfig.nebius.image.tooltip"
        />
        <LabeledInput
          v-else
          :mode="mode"
          :disabled="disabled"
          :value="value.imageFamily"
          label-key="cluster.machineConfig.nebius.imageFamily.label"
          placeholder-key="cluster.machineConfig.nebius.imageFamily.placeholder"
          tooltip-key="cluster.machineConfig.nebius.image.tooltip"
          @change="
            (e) => {
              value.imageFamily = e.target.value;
            }
          "
        />
      </div>
    </div>

    <div class="row mb-20">
      <div class="col span-6">
        <LabeledSelect
          v-if="imageOptions"
          v-model:value="value.imageId"
          :mode="mode"
          :disabled="disabled"
          :options="imageOptions"
          :searchable="true"
          label-key="cluster.machineConfig.nebius.imageId.label"
          tooltip-key="cluster.machineConfig.nebius.image.tooltip"
        />
        <LabeledInput
          v-else
          :mode="mode"
          :disabled="disabled"
          :value="value.imageId"
          label-key="cluster.machineConfig.nebius.imageId.label"
          placeholder-key="cluster.machineConfig.nebius.imageId.placeholder"
          tooltip-key="cluster.machineConfig.nebius.image.tooltip"
          @change="
            (e) => {
              value.imageId = e.target.value;
            }
          "
        />
      </div>
      <div class="col span-6">
        <LabeledInput
          type="number"
          :mode="mode"
          :disabled="disabled"
          min="1"
          step="1"
          :value="value.diskSizeGib"
          label-key="cluster.machineConfig.nebius.diskSizeGib.label"
          required
          @change="
            (e) => {
              value.diskSizeGib = e.target.value;
            }
          "
        />
      </div>
    </div>

    <div class="row mb-20">
      <div class="col span-6">
        <LabeledSelect
          v-model:value="value.diskType"
          :mode="mode"
          :disabled="disabled"
          :options="diskTypeOptions"
          label-key="cluster.machineConfig.nebius.diskType.label"
        />
      </div>
      <div class="col span-6">
        <LabeledSelect
          v-model:value="value.publicIp"
          :mode="mode"
          :disabled="disabled"
          :options="publicIpOptions"
          label-key="cluster.machineConfig.nebius.publicIp.label"
          tooltip-key="cluster.machineConfig.nebius.publicIp.tooltip"
        />
      </div>
    </div>

    <h3>
      <t k="cluster.machineConfig.nebius.network.header" />
    </h3>

    <div class="row mb-20">
      <div class="col span-12">
        <LabeledSelect
          v-if="securityGroupOptions"
          v-model:value="value.securityGroups"
          :mode="mode"
          :disabled="disabled"
          :options="securityGroupOptions"
          :multiple="true"
          :searchable="true"
          :close-on-select="false"
          label-key="cluster.machineConfig.nebius.securityGroups.label"
          tooltip-key="cluster.machineConfig.nebius.securityGroups.tooltip"
        />
        <LabeledInput
          v-else
          :mode="mode"
          :disabled="disabled"
          :value="(value.securityGroups || []).join(',')"
          label-key="cluster.machineConfig.nebius.securityGroups.label"
          placeholder-key="cluster.machineConfig.nebius.securityGroups.placeholder"
          tooltip-key="cluster.machineConfig.nebius.securityGroups.tooltip"
          @change="
            (e) => {
              value.securityGroups = splitList(e.target.value);
            }
          "
        />
      </div>
    </div>

    <h3>
      <t k="cluster.machineConfig.nebius.ssh.header" />
    </h3>

    <div class="row mb-20">
      <div class="col span-6">
        <LabeledInput
          :mode="mode"
          :disabled="disabled"
          :value="value.sshUser"
          label-key="cluster.machineConfig.nebius.sshUser.label"
          required
          @change="
            (e) => {
              value.sshUser = e.target.value;
            }
          "
        />
      </div>
      <div class="col span-6">
        <LabeledInput
          type="number"
          :mode="mode"
          :disabled="disabled"
          min="1"
          max="65535"
          step="1"
          :value="value.sshPort"
          label-key="cluster.machineConfig.nebius.sshPort.label"
          @change="
            (e) => {
              value.sshPort = e.target.value;
            }
          "
        />
      </div>
    </div>

    <portal :to="`advanced-${ uuid }`">
      <h3>
        <t k="cluster.machineConfig.nebius.advanced.header" />
      </h3>

      <div class="mb-20">
        <Checkbox
          :mode="mode"
          :disabled="disabled"
          :value="value.preemptible"
          label-key="cluster.machineConfig.nebius.preemptible.label"
          @click="
            () => {
              value.preemptible = !value.preemptible;
            }
          "
        />
        <Banner v-if="value.preemptible" class="mt-10" color="warning">
          {{ t("cluster.machineConfig.nebius.preemptible.warning") }}
        </Banner>
      </div>

      <div class="row mb-20">
        <div class="col span-6">
          <LabeledInput
            :mode="mode"
            :disabled="disabled"
            :value="value.createTimeout"
            label-key="cluster.machineConfig.nebius.createTimeout.label"
            tooltip-key="cluster.machineConfig.nebius.timeout.tooltip"
            placeholder="20m"
            @change="
              (e) => {
                value.createTimeout = e.target.value;
              }
            "
          />
        </div>
        <div class="col span-6">
          <LabeledInput
            :mode="mode"
            :disabled="disabled"
            :value="value.deleteTimeout"
            label-key="cluster.machineConfig.nebius.deleteTimeout.label"
            tooltip-key="cluster.machineConfig.nebius.timeout.tooltip"
            placeholder="5m"
            @change="
              (e) => {
                value.deleteTimeout = e.target.value;
              }
            "
          />
        </div>
      </div>
    </portal>
  </div>
</template>

<style lang="scss" scoped>
</style>
