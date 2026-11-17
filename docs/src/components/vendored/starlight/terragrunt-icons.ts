import { Icons as StarlightIcons } from './Icons';
import { definitions as starlightDefinitions } from './file-tree-icons';
import type { Definitions } from './file-tree-processor';

// Starlight's file tree icons, plus Terragrunt and OpenTofu icons for their files.
export const Icons = {
	...StarlightIcons,
	'custom:terragrunt':
		'<style>.icon-light{display:none}.icon-dark{display:block}:root[data-theme=\'light\'] .icon-light{display:block}:root[data-theme=\'light\'] .icon-dark{display:none}</style><g class="icon-light"><path fill-rule="evenodd" clip-rule="evenodd" d="M12.0404 0.513672L22.4616 6.514V18.514L12.0404 24.514L1.61914 18.514V6.514L12.0404 0.513672ZM21.3366 6.832L12.0404 1.35L2.74414 6.832V18.696L12.0404 23.178L21.3366 18.696V6.832Z" fill="#160C56"></path><path fill-rule="evenodd" clip-rule="evenodd" d="M12.0404 23.179V18.697V12.515L7.39226 9.423L2.74414 6.332V12.515V18.697L12.0404 23.179Z" fill="white"></path><path d="M16.6885 20.488L21.3366 18.697L16.6885 15.606L12.0404 18.697V23.179L16.6885 20.488Z" fill="#87E0E1"></path><path d="M12.0404 12.515V18.697L16.6885 15.606L12.0404 12.515Z" fill="#1B46DD"></path><path d="M16.6885 9.423L12.0404 12.515L16.6885 15.606L21.3366 12.515L16.6885 9.423Z" fill="#B068E9"></path><path d="M16.6885 4.241L12.0404 1.15L7.39226 4.241L2.74414 6.332L7.39226 9.423L12.0404 6.332L16.6885 9.423L21.3366 6.332L16.6885 4.241Z" fill="#F9DB4E"></path><path d="M12.0404 12.515L16.6885 9.423L12.0404 6.332L7.39226 9.423L12.0404 12.515Z" fill="#E94A5D"></path></g><g class="icon-dark"><path fill-rule="evenodd" clip-rule="evenodd" d="M12.0404 0.513672L22.4616 6.514V18.514L12.0404 24.514L1.61914 18.514V6.514L12.0404 0.513672ZM21.3366 6.832L12.0404 1.35L2.74414 6.832V18.696L12.0404 23.178L21.3366 18.696V6.832Z" fill="white"></path><path fill-rule="evenodd" clip-rule="evenodd" d="M12.0404 23.179V18.697V12.515L7.39226 9.423L2.74414 6.332V12.515V18.697L12.0404 23.179Z" fill="white"></path><path d="M16.6885 20.488L21.3366 18.697L16.6885 15.606L12.0404 18.697V23.179L16.6885 20.488Z" fill="#87E0E1"></path><path d="M12.0404 12.515V18.697L16.6885 15.606L12.0404 12.515Z" fill="#1B46DD"></path><path d="M16.6885 9.423L12.0404 12.515L16.6885 15.606L21.3366 12.515L16.6885 9.423Z" fill="#B068E9"></path><path d="M16.6885 4.241L12.0404 1.15L7.39226 4.241L2.74414 6.332L7.39226 9.423L12.0404 6.332L16.6885 9.423L21.3366 6.332L16.6885 4.241Z" fill="#F9DB4E"></path><path d="M12.0404 12.515L16.6885 9.423L12.0404 6.332L7.39226 9.423L12.0404 12.515Z" fill="#E94A5D"></path></g>',
	'custom:opentofu':
		'<g><path d="M11.8144 1.3393C11.9303 1.27492 12.0694 1.27492 12.1853 1.3393L20.4151 5.90989C20.6817 6.05619 20.6817 6.44244 20.4151 6.58875L12.1853 11.1593C12.0694 11.2237 11.9303 11.2237 11.8144 11.1593L3.58471 6.58875C3.31812 6.44244 3.31812 6.05619 3.58471 5.90989L11.8144 1.3393Z" fill="#E6C220"></path><path d="M21.1505 7.65992C21.4055 7.51947 21.7184 7.70674 21.7184 7.99935V17.1347C21.7184 17.2751 21.6431 17.4097 21.5214 17.4741L13.2105 22.0857C12.9555 22.2261 12.6426 22.0389 12.6426 21.7462V12.6109C12.6426 12.4705 12.7179 12.3359 12.8396 12.2715L21.1505 7.65992Z" fill="white"></path><path fill-rule="evenodd" clip-rule="evenodd" d="M2.2793 7.99935C2.2793 7.70674 2.59226 7.51947 2.84726 7.65992L11.1581 12.2715C11.2798 12.3417 11.3552 12.4705 11.3552 12.6109V21.7462C11.3552 22.0389 11.0422 22.2261 10.7872 22.0857L2.48214 17.48C2.36043 17.4097 2.28509 17.281 2.28509 17.1405V7.99935H2.2793ZM5.76958 15.1706V15.1823L3.82227 14.1464V14.1347C3.86863 13.5202 4.33807 13.2569 4.87706 13.5436C5.41605 13.8304 5.81594 14.5561 5.76958 15.1706ZM8.83599 16.9655V16.9538C8.88235 16.3393 8.48245 15.6136 7.94347 15.3268C7.40448 15.0401 6.93504 15.3034 6.88867 15.9179V15.9296L8.83599 16.9655Z" fill="#FEDA15"></path></g>',
};

export type FileTreeIcon = keyof typeof Icons;

export const definitions: Definitions<FileTreeIcon> = {
	files: {
		...starlightDefinitions.files,
		'.terraform.lock.hcl': 'custom:opentofu',
	},
	extensions: {
		...starlightDefinitions.extensions,
		'.hcl': 'custom:terragrunt',
		'.tf': 'custom:opentofu',
		'.tf.json': 'custom:opentofu',
		'.tfvars': 'custom:opentofu',
		'.tfvars.json': 'custom:opentofu',
		'.tfplan': 'custom:opentofu',
	},
	partials: starlightDefinitions.partials,
};
